// Copyright (c) 2021 Alexandru Catrina <alex@codeissues.net>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
//    contributors may be used to endorse or promote products derived from
//    this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lexndru/expenses"

	"gorm.io/gorm"
)

type journal struct {
	dbInstance  *gorm.DB
	dbBatchSize int
}

func (j journal) Setup(router *mux.Router) {
	router.HandleFunc("/journal/{signature:[a-z0-9-]+}", j.evaluateWithoutOutput).Methods(http.MethodHead)
	router.HandleFunc("/journal/{signature:[a-z0-9-]+}", j.evaluate).Methods(http.MethodGet)
	router.HandleFunc("/journal/{signature:[a-z0-9-]+}", j.analyze).Methods(http.MethodPost)
	router.HandleFunc("/journal/{signature:[a-z0-9-]+}/download", j.download).Methods(http.MethodGet)
}

func (j journal) download(wr http.ResponseWriter, rq *http.Request) {
	response := Response{wr}
	params := mux.Vars(rq)

	signature := params["signature"]
	startTime := time.Now()

	buf := &bytes.Buffer{}
	out := csv.NewWriter(buf)

	if rs, ok := memory[signature]; ok {
		for _, record := range rs.records {
			out.Write(record.ToSlice())
		}
	}

	out.Flush() // finish writing
	response.OkayStream(buf.Bytes(), true, time.Since(startTime), rq)
}

func (j journal) evaluateWithoutOutput(wr http.ResponseWriter, rq *http.Request) {
	response := Response{wr}
	startTime := time.Now()

	params := mux.Vars(rq)
	signature := params["signature"]

	if err := _evaluate(j, signature); err != nil {
		response.Fault(err, rq)
	} else {
		response.Okay(nil, false, time.Since(startTime), rq)
	}
}

func (j journal) evaluate(wr http.ResponseWriter, rq *http.Request) {
	response := Response{wr}
	startTime := time.Now()

	params := mux.Vars(rq)
	signature := params["signature"]

	if err := _evaluate(j, signature); err != nil {
		response.Fault(err, rq)
	} else {
		if cache, ok := memory[signature]; ok {
			if out, err := json.Marshal(cache.patterns); err != nil {
				response.Fault(err, rq)
			} else {
				response.Okay(out, false, time.Since(startTime), rq)
			}
		} else {
			response.Okay([]byte("{}"), false, -1, rq)
		}
	}
}

func (j journal) analyze(wr http.ResponseWriter, rq *http.Request) {
	response := Response{wr}
	startTime := time.Now()

	params := mux.Vars(rq)
	signature := params["signature"]

	if payload, err := ioutil.ReadAll(rq.Body); err != nil { // TODO: avoid ioutil because of memory issues?
		response.Fault(err, rq)
	} else {
		var records collection
		if err := json.Unmarshal(payload, &records); err != nil {
			response.Fault(err, rq)
		} else if len(records) > 0 {
			sort.Slice(records, func(i, j int) bool {
				return records[i].Date.After(records[j].Date)
			})

			newestRecord := records[0].Date
			oldestRecord := records[len(records)-1].Date

			var reg expenses.Transactions
			ctx := expenses.PullContext{
				Storage: j.dbInstance.Where("signature = ? and date between ? and ?", signature, oldestRecord, newestRecord),
				Limit:   j.dbBatchSize,
			}

			if err := reg.Pull(ctx); err != nil {
				response.Fault(err, rq)
			} else {
				research := _research(reg, records, signature)
				if out, err := json.Marshal(research); err != nil {
					response.Fault(err, rq)
				} else {
					response.Okay(out, false, time.Since(startTime), rq)
				}
			}
		} else {
			response.Okay([]byte("[]"), false, time.Since(startTime), rq)
		}
	}
}

type similarity struct {
	Grade  int                  `json:"grade"`
	Mirror record               `json:"record"`
	Parent expenses.Transaction `json:"parent"`
}

type statement struct {
	record

	Calculated map[string]pointbus `json:"$calculated"`
	Similarity []similarity        `json:"$similarity"`
}

func _research(reg expenses.Transactions, records collection, signature string) []statement {
	var statements = make([]statement, len(records))

	for index, record := range records {
		statements[index].record = record // inherit?
		statements[index].Calculated = make(map[string]pointbus)
		statements[index].Similarity = make([]similarity, 0)

		// look for duplicate or similar transactions
		for _, transaction := range reg {
			if g := record.CompareWithTransaction(transaction); g > 0 {
				sim := similarity{
					Grade:  g,
					Mirror: _fromTransaction(transaction),
					Parent: transaction,
				}

				statements[index].Similarity = append(statements[index].Similarity, sim)
			}
		}

		rs, ok := memory[signature]
		if !ok {
			continue
		}

		// look for labels based on previous calculated patterns
		if features, ok := rs.patterns[record.Party()]; ok {
			score := make(map[string]pointbus) // keep track of feature/label points

			for _, feature := range features {
				var points pointbus

				if len(feature.Amounts) == 0 {
					log.Printf("warning: feature \"%s\" has no amounts\n", feature.Category)
					continue // should not be the case, but best to be safe than crash
				}

				if (feature.Polarity[0] == 0 && record.Amount > 0) || (feature.Polarity[1] == 0 && record.Amount < 0) {
					continue // feature is not about this record because polarity for in/out is against amount sign
				}

				absValue := int(record.Amount)
				if absValue < 0 {
					absValue *= -1
				}

				if len(feature.Amounts) > 1 {
					if _isBetweenAmountDeviation(feature.Amounts, absValue) {
						points[0] += 1
					}
				} else if _isBetweenAmountAprox(feature.Amounts[0], absValue) {
					points[1] += 1
				}

				if feature.HasMonth(record.Date.Month()) {
					points[2] += 1
				}

				if feature.HasWeekday(record.Date.Weekday()) {
					points[3] += 1
				}

				if feature.HasDay(record.Date.Day()) {
					points[4] += 1
				}

				if _total(points[:]...) == 0 {
					continue // no points means this feature is not what we need
				}

				// append polarity avg. as popolarity measurement
				points[5] += (feature.Polarity[0] + feature.Polarity[1]) / 2

				if accPoints, ok := score[feature.Category]; ok {
					var totalPoints pointbus
					for i := 0; i < cap(totalPoints); i++ {
						totalPoints[i] = points[i] + accPoints[i]
					}
					score[feature.Category] = totalPoints
				} else {
					score[feature.Category] = points
				}
			}

			statements[index].Calculated = score
		}
	}

	return statements
}

type collection []record

type record struct {
	Sender   string    `json:"sender"`
	Receiver string    `json:"receiver"`
	Label    string    `json:"label"`
	Date     time.Time `json:"date"`
	Amount   int64     `json:"amount"`
	Parent   string    `json:"parent"`
}

func (r record) ToSlice() []string {
	var out = make([]string, 6)

	out[0] = r.Sender
	out[1] = r.Receiver
	out[2] = r.Label
	out[3] = fmt.Sprintf("%d", r.Date.Unix())
	out[4] = fmt.Sprintf("%d", r.Amount)
	out[5] = r.Parent

	return out
}

func (r record) ToFeature() feature {
	polarity := [2]int{0, 0} // in, out
	absValue := int(r.Amount)

	if absValue < 0 {
		absValue *= -1
		polarity[1] += 1
	} else {
		polarity[0] += 1
	}

	return feature{
		Category: r.Label,
		Polarity: polarity,
		Amounts:  []int{absValue},
		Weekdays: []time.Weekday{r.Date.Weekday()},
		Months:   []time.Month{r.Date.Month()},
		Days:     []int{r.Date.Day()},
	}
}

func (r record) Party() string {
	return fmt.Sprintf("sender=%s receiver=%s", r.Sender, r.Receiver)
}

func (r record) CompareWith(r2 record) int {
	var grade int // similarity grade

	if r2.Amount == r.Amount && r2.Date.Equal(r.Date) {
		if r2.Sender == r.Sender {
			grade += 1
		}

		if r2.Receiver == r.Receiver {
			grade += 2
		}

		if r2.Label == r.Label {
			grade += 4
		}
	}

	return grade
}

func (r record) CompareWithTransaction(t expenses.Transaction) int {
	var grade int // similarity grade

	if t.Amount == r.Amount && t.Date.Equal(r.Date) {
		senderName := t.SenderName
		if name, ok := _fromHeaders(t.Headers, "sender="); ok {
			senderName = name
		}

		if senderName == r.Sender {
			grade += 1
		}

		receiverName := t.ReceiverName
		if name, ok := _fromHeaders(t.Headers, "receiver="); ok {
			receiverName = name
		}

		if receiverName == r.Receiver {
			grade += 2
		}

		if t.LabelName == r.Label {
			grade += 4
		}
	}

	return grade
}

type feature struct {
	Category string
	Polarity [2]int
	Amounts  []int
	Weekdays []time.Weekday
	Months   []time.Month
	Days     []int
}

func (f *feature) Update(r record) error {
	if r.Label != f.Category {
		return fmt.Errorf("cannot update feature `%s` with record from another category `%s`", f.Category, r.Label)
	}

	absValue := int(r.Amount)
	if absValue < 0 {
		absValue *= -1
		f.Polarity[1] += 1
	} else {
		f.Polarity[0] += 1
	}

	f.Amounts = append(f.Amounts, absValue)
	sort.Ints(f.Amounts)

	if day := r.Date.Weekday(); !f.HasWeekday(day) {
		f.Weekdays = append(f.Weekdays, day)
		sort.SliceStable(f.Weekdays, func(i, j int) bool {
			return f.Weekdays[i] < f.Weekdays[j]
		})
	}

	if day := r.Date.Day(); !f.HasDay(day) {
		f.Days = append(f.Days, day)
		sort.SliceStable(f.Days, func(i, j int) bool {
			return f.Days[i] < f.Days[j]
		})
	}

	return nil
}

func (f feature) HasMonth(month time.Month) bool {
	for _, m := range f.Months {
		if m == month {
			return true
		}
	}

	return false
}

func (f feature) HasWeekday(weekday time.Weekday) bool {
	for _, w := range f.Weekdays {
		if w == weekday {
			return true
		}
	}

	return false
}

func (f feature) HasDay(day int) bool {
	for _, d := range f.Days {
		if d == day {
			return true
		}
	}

	return false
}

type pointbus [6]int

type routines map[string]map[string][]feature

type tendency map[string][]feature // flatten routines

type results struct {
	records  collection
	patterns tendency
}

var memory = make(map[string]results)

func _evaluate(j journal, signature string) error {
	var reg expenses.Transactions

	ctx := expenses.PullContext{
		Storage: j.dbInstance.Where("signature = ?", signature),
		Limit:   j.dbBatchSize,
	}

	if err := reg.Pull(ctx); err != nil {
		return err
	}

	var records = make(collection, 0, cap(reg))

	for _, trx := range reg {
		if len(trx.Details) > 0 {
			details := make(collection, len(trx.Details))

			var total int64
			for idx, dls := range trx.Details {
				shadowRecord := _fromTransaction(trx)
				shadowRecord.Label = dls.LabelName // use details label
				shadowRecord.Amount = dls.Amount   // use details amount

				if trx.Amount < 0 {
					shadowRecord.Amount = shadowRecord.Amount * -1
				}

				details[idx] = shadowRecord
				total += shadowRecord.Amount
			}
			// validated amount breakdown to prevent corrupted transactions
			if trx.Amount == total {
				records = append(records, details...)
			}
		} else {
			records = append(records, _fromTransaction(trx))
		}
	}

	memory[signature] = results{
		records:  records,
		patterns: _compute(records),
	}

	return nil
}

func _fromHeaders(headers string, keyword string) (string, bool) {
	kwSize := len(keyword)

	for _, token := range strings.Fields(headers) {
		if index := strings.Index(token, keyword); index == 0 {
			return token[kwSize:], true
		}
	}

	return "", false
}

func _fromTransaction(t expenses.Transaction) record {
	senderName := t.SenderName
	if name, ok := _fromHeaders(t.Headers, "sender="); ok {
		senderName = name
	}

	receiverName := t.ReceiverName
	if name, ok := _fromHeaders(t.Headers, "receiver="); ok {
		receiverName = name
	}

	return record{
		Sender:   senderName,
		Receiver: receiverName,
		Label:    t.LabelName,
		Date:     t.Date,
		Amount:   t.Amount,
		Parent:   *t.UUID,
	}
}

func _isBetweenAmountDeviation(amounts []int, value int) bool {
	var allAmounts = make([]int, len(amounts))
	copy(allAmounts, amounts)

	allAmounts = append(allAmounts, value)
	sort.Ints(allAmounts)

	deviation := make([]float64, len(allAmounts)-1)
	for i := 0; i < len(allAmounts)-1; i++ {
		a, b := allAmounts[i], allAmounts[i+1]
		deviation[i] = float64(b-a) / float64(b)
	}

	calcAmountsMedian := func(ns []int) int {
		if len(ns)%2 == 1 {
			return ns[len(ns)/2]
		}

		nextIndex := len(ns) / 2
		prevIndex := nextIndex - 1

		return (ns[prevIndex] + ns[nextIndex]) / 2
	}

	calcDeviationMedian := func(ns []float64) float64 {
		if len(ns)%2 == 1 {
			return ns[len(ns)/2]
		}

		nextIndex := len(ns) / 2
		prevIndex := nextIndex - 1

		return (ns[prevIndex] + ns[nextIndex]) / 2
	}

	amountMedian := float64(calcAmountsMedian(allAmounts))
	deviationMedian := calcDeviationMedian(deviation)

	dif := amountMedian * deviationMedian
	min := amountMedian - dif
	max := amountMedian + dif

	return int(min) <= value && value <= int(max)
}

func _isBetweenAmountAprox(amount int, value int) bool {
	digits := len(fmt.Sprintf("%d", amount)) - 2 // -2 point decimals

	pow10 := float64(_power(10, digits))
	ratio := float64(amount) / pow10

	min := math.Floor(ratio) * pow10
	max := math.Ceil(ratio) * pow10

	return int(min) <= value && value <= int(max)
}

const UNNAMED_ENTRY = "?"

func _compute(records []record) tendency {
	model := make(routines)

	for i := len(records) - 1; i > 0; i-- {
		if record := records[i]; record.Label != UNNAMED_ENTRY {
			// TODO: refactor the calculate part into something less procedural?
			_calculate(model, record.Party(), records[i-1].Date.Month(), record)
		}
	}

	conclusion := make(tendency)
	for party, categories := range model {
		conclusion[party] = make([]feature, 0)
		for _, features := range categories {
			conclusion[party] = append(conclusion[party], features...)
		}
	}

	return conclusion
}

func _calculate(model routines, party string, nextMonth time.Month, record record) {
	// TODO: how can I refactor this?
	if categories, ok := model[party]; ok {
		if features, ok := categories[record.Label]; ok {
			lastFeature := features[0]
			// check if expenses are recurrent or there's a gap between
			lastMonth := lastFeature.Months[0]
			if lastMonth+1 == nextMonth || (lastMonth == 12 && nextMonth == 1) {
				lastFeature.Months = append([]time.Month{nextMonth}, lastFeature.Months...)
				// ^^^ perhaps circular compute is too wasteful?
				if err := lastFeature.Update(record); err != nil {
					log.Printf("warning: calculate features error: %s\n", err)
				} else {
					model[party][record.Label][0] = lastFeature
				}
			} else if record.Date.Month() == lastMonth {
				if err := lastFeature.Update(record); err != nil {
					log.Printf("warning: calculate features error: %s\n", err)
				} else {
					model[party][record.Label][0] = lastFeature
				}
			} else {
				model[party][record.Label] = append([]feature{record.ToFeature()}, features...)
			}
		} else {
			model[party][record.Label] = make([]feature, 1)
			model[party][record.Label][0] = record.ToFeature()
		}
	} else {
		model[party] = make(map[string][]feature, 1)
		model[party][record.Label] = make([]feature, 1)
		model[party][record.Label][0] = record.ToFeature()
	}
}

func _total(ns ...int) int {
	var total int

	for _, n := range ns {
		total += n
	}

	return total
}

func _power(num, exp int) int {
	if exp == 0 {
		return 1
	}

	result := num
	for i := 2; i <= exp; i++ {
		result *= num
	}

	return result
}
