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
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/lexndru/expenses"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	registryDBInstance = sqlite.Open("file::memory:")
	registryHttpRouter *mux.Router

	jsonActors []byte
	dataActors expenses.Actors

	jsonLabels []byte
	dataLabels expenses.Labels

	jsonTransactions []byte
	dataTransactions expenses.Transactions
)

func init() {
	if db, err := gorm.Open(registryDBInstance, &gorm.Config{}); err != nil {
		panic(err)
	} else {
		registryHttpRouter = mux.NewRouter()

		if err := expenses.Install(db); err != nil {
			panic(err)
		}

		mod := registry{db /* Debug() */, 10}
		mod.Setup(registryHttpRouter)
	}

	jsonActors, _ = ioutil.ReadFile("examples/test_actors.json")
	if err := expenses.FromJson(jsonActors, &dataActors); err != nil {
		panic(err)
	}

	jsonLabels, _ = ioutil.ReadFile("examples/test_labels.json")
	if err := expenses.FromJson(jsonLabels, &dataLabels); err != nil {
		panic(err)
	}

	jsonTransactions, _ = ioutil.ReadFile("examples/test_transactions.json")
	if err := expenses.FromJson(jsonTransactions, &dataTransactions); err != nil {
		panic(err)
	}

	log.SetOutput(ioutil.Discard)
}

func TestReadJsonActors(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/actors", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on first GET but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else if string(body) != "[]" {
		t.Fatal("Expected empty json list on first call")
	}
}

func TestWriteJsonActors(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/actors", bytes.NewReader(jsonActors)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataActors)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example actors after POST call")
		}
	}
}

func TestReadJsonActorsAfterWrite(t *testing.T) {
	crono := time.Now()
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/actors", nil))
	reply := buf.Result()
	speedtest := time.Since(crono)

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataActors)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example actors after POST call")
		}
	}

	crono2 := time.Now()
	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/actors", nil))
	reply2 := buf2.Result()
	speedtest2 := time.Since(crono2)

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after GET but instead got %v\n", reply2.StatusCode)
	}

	if speedtest < speedtest2 {
		t.Fatalf("Expected third GET to be faster with cache, but it's not: %v v %v\n", speedtest, speedtest2)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataActors)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example actors after cache call")
		}
	}
}

func TestDuplicateJsonActors(t *testing.T) {
	actorDuplicate := expenses.Actors{dataActors[0]}
	newJson, err := expenses.ToJson(actorDuplicate)
	if err != nil {
		t.Fatal(err)
	}

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/actors", bytes.NewReader(newJson)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !bytes.Equal(newJson, body) {
			t.Fatal("Expected json list with example actors after POST call")
		}
	}

	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/actors", nil))
	reply2 := buf2.Result()

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on last GET but instead got %v\n", reply2.StatusCode)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataActors)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example actors after two POST calls with duplicates")
		}
	}
}

func TestWriteWrongJsonActors(t *testing.T) {
	wrongPayload := []byte(`{"key":"actor name"}`)

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/actors", bytes.NewReader(wrongPayload)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request after POST with wrong payload but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !strings.HasPrefix(string(body), "json: cannot unmarshal") {
			t.Fatalf("Expected error about json unmarshal but instead got: %v\n", string(body))
		}
	}
}

func TestReadJsonLabels(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/labels", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on first GET but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else if string(body) != "[]" {
		t.Fatal("Expected empty json list on first call")
	}
}

func TestWriteJsonLabels(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/labels", bytes.NewReader(jsonLabels)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataLabels)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example actors after POST call")
		}
	}
}

func TestReadJsonLabelsAfterWrite(t *testing.T) {
	crono := time.Now()
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/labels", nil))
	reply := buf.Result()
	speedtest := time.Since(crono)

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataLabels)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example labels after POST call")
		}
	}

	crono2 := time.Now()
	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/labels", nil))
	reply2 := buf2.Result()
	speedtest2 := time.Since(crono2)

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after GET but instead got %v\n", reply2.StatusCode)
	}

	if speedtest < speedtest2 {
		t.Fatalf("Expected third GET to be faster with cache, but it's not: %v v %v\n", speedtest, speedtest2)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataLabels)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example labels after cache call")
		}
	}
}

func TestDuplicateJsonLabels(t *testing.T) {
	// BUG: expenses labels push overwrites a parent's parent with NULL if the following goes:
	//
	//      push lb=x p_lb=y
	//      sql -> lb=x p_lb=y
	//      sql -> lb=y p_lb=NULL
	//      --------------------- ok because first time for X and Y
	//
	//      push lb=z p_lb=x
	//      sql -> lb=z p_lb=x
	//      sql -> lb=x p_lb=NULL
	//      --------------------- wrong! because x already exists and has parent
	//
	// Workaround is to send the tree of parents when push call
	labelDuplicate := expenses.Labels{
		{
			Name:       "Label #1",
			ParentName: expenses.NullString{sql.NullString{String: "Label #5", Valid: true}},
		},
		// {
		// 	Name:       "Label #5",
		// 	ParentName: expenses.NullString{sql.NullString{String: "Label #0", Valid: true}}, // THIS SHOULD NOT BE NEEDED !
		// },
	}
	newJson, err := expenses.ToJson(labelDuplicate)
	if err != nil {
		t.Fatal(err)
	}

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/labels", bytes.NewReader(newJson)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !bytes.Equal(newJson, body) {
			t.Fatal("Expected json list with example labels after POST call")
		}
	}

	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/labels", nil))
	reply2 := buf2.Result()

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on last GET but instead got %v\n", reply2.StatusCode)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		probe, _ := expenses.ToJson(dataLabels)
		if !bytes.Equal(probe, body) {
			t.Fatal("Expected json list with example labels after two POST calls with duplicates")
		}
	}
}

func TestWriteWrongJsonLabels(t *testing.T) {
	wrongPayload := []byte(`{"key":"label name"}`)

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/labels", bytes.NewReader(wrongPayload)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request after POST with wrong payload but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !strings.HasPrefix(string(body), "json: cannot unmarshal") {
			t.Fatalf("Expected error about json unmarshal but instead got: %v\n", string(body))
		}
	}
}

func _compareTransactions(a, b expenses.Transactions) error {
	if len(a) != len(b) {
		return fmt.Errorf("Not the same size: %v/%v\n", len(a), len(b))
	}

	for i, trx := range a {
		if trx.Date != b[i].Date {
			return fmt.Errorf("dates are different: %v/%v\n", trx.Date, b[i].Date)
		}

		if trx.Amount != b[i].Amount {
			return fmt.Errorf("amounts are different: %v/%v", trx.Amount, b[i].Amount)
		}

		if trx.LabelName != b[i].LabelName {
			return fmt.Errorf("labels are different: %v/%v", trx.LabelName, b[i].LabelName)
		}

		if trx.SenderName != b[i].SenderName {
			return fmt.Errorf("senders are different: %v/%v", trx.SenderName, b[i].SenderName)
		}

		if trx.ReceiverName != b[i].ReceiverName {
			return fmt.Errorf("receivers are different: %v/%v", trx.ReceiverName, b[i].ReceiverName)
		}

		for j, ls := range trx.Details {
			if ls.Amount != b[i].Details[j].Amount {
				return fmt.Errorf("details %d amount different: %v/%v", j, ls.Amount, b[i].Details[j].Amount)
			}

			if ls.LabelName != b[i].Details[j].LabelName {
				return fmt.Errorf("details %d label are different: %v/%v", j, ls.LabelName, b[i].Details[j].LabelName)
			}
		}
	}

	return nil
}

func TestReadJsonTransactions(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/transactions", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on first GET but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else if string(body) != "[]" {
		t.Fatal("Expected empty json list on first call")
	}
}

func TestWriteJsonTransactions(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/transactions", bytes.NewReader(jsonTransactions)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}

		if err := _compareTransactions(dataTransactions, probeTrx); err != nil {
			t.Fatal("Expected json list with example transactions after POST call")
		}
	}
}

func TestReadJsonTransactionsAfterWrite(t *testing.T) {
	crono := time.Now()
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/transactions", nil))
	reply := buf.Result()
	speedtest := time.Since(crono)

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}

		if err := _compareTransactions(dataTransactions, probeTrx); err != nil {
			t.Fatalf("Expected json list with example transactions after POST call: %s", err)
		}
	}

	crono2 := time.Now()
	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/transactions", nil))
	reply2 := buf2.Result()
	speedtest2 := time.Since(crono2)

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on GET after GET but instead got %v\n", reply2.StatusCode)
	}

	if speedtest < speedtest2 {
		t.Fatalf("Expected third GET to be faster with cache, but it's not: %v v %v\n", speedtest, speedtest2)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}

		if err := _compareTransactions(dataTransactions, probeTrx); err != nil {
			t.Fatal("Expected json list with example transactions after POST call")
		}
	}
}

func TestDuplicateJsonTransctions(t *testing.T) {
	date, _ := time.Parse("2006-01-02", "2021-04-29")
	trxKey := uuid.New().String()
	newTrx := expenses.Transactions{
		{
			UUID:         &trxKey,
			Date:         date,
			Amount:       12345,
			LabelName:    "Label new",
			SenderName:   "Actor sender",
			ReceiverName: "Actor receiver",
			// NOTE: details from json is null if not provided here
		},
	}

	newJson, err := expenses.ToJson(newTrx)
	if err != nil {
		t.Fatal(err)
	}

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/transactions", bytes.NewReader(newJson)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !bytes.Equal(newJson, body) {
			t.Fatal("Expected json list with example transactions after POST call")
		}
	}

	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/transactions", nil))
	reply2 := buf2.Result()

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on last GET but instead got %v\n", reply2.StatusCode)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}
		if len(probeTrx) == len(dataTransactions) {
			t.Fatal("Expected probe to have +1 transaction after write")
		}
		recent := probeTrx[0]
		if err := _compareTransactions(expenses.Transactions{recent}, newTrx); err != nil {
			t.Fatal(err)
		}
	}

	retryTrx := newTrx
	retryTrx[0].UUID = nil

	newJson2, err := expenses.ToJson(retryTrx)
	if err != nil {
		t.Fatal(err)
	}

	buf3 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf3, httptest.NewRequest("POST", "/registry/transactions", bytes.NewReader(newJson2)))
	reply3 := buf3.Result()

	if reply3.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK after POST but instead got %v\n", reply3.StatusCode)
	}

	if body, err := io.ReadAll(reply3.Body); err != nil {
		t.Fatal(err)
	} else {
		var probe expenses.Transactions
		if err := expenses.FromJson(body, &probe); err != nil {
			t.Fatal(err)
		}
		if err := _compareTransactions(probe, retryTrx); err != nil {
			t.Fatal("Expected json list with example transactions after POST call")
		}
	}

	buf4 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf4, httptest.NewRequest("GET", "/registry/transactions", nil))
	reply4 := buf4.Result()

	if reply4.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on last GET but instead got %v\n", reply4.StatusCode)
	}
	if body, err := io.ReadAll(reply4.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}
		if len(probeTrx) != len(dataTransactions)+2 {
			t.Fatal("Expected probe to have +2 transaction after 2 write")
		}
	}
}

func TestWriteWrongJsonTransactions(t *testing.T) {
	wrongPayload := []byte(`{"key":"trx amount"}`)

	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/transactions", bytes.NewReader(wrongPayload)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request after POST with wrong payload but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		if !strings.HasPrefix(string(body), "json: cannot unmarshal") {
			t.Fatalf("Expected error about json unmarshal but instead got: %v\n", string(body))
		}
	}
}
func TestReadWrongParamsJsonTransactions(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/transactions/2021/x", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found for GET with wrong params but instead got %v\n", reply.StatusCode)
	}

	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/transactions/x/1", nil))
	reply2 := buf2.Result()

	if reply2.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 Bad Request for GET with wrong params but instead got %v\n", reply.StatusCode)
	}
}

func TestReadJsonMonthlyTransactions(t *testing.T) {
	buf := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/registry/transactions/2021/03", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for GET on monthly filter but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}

		if len(probeTrx) != 0 {
			t.Fatalf("Expected 0 transaction for year 2021 month 03 since there's no feed but got: %v\n", len(probeTrx))
		}
	}

	buf2 := httptest.NewRecorder()
	registryHttpRouter.ServeHTTP(buf2, httptest.NewRequest("GET", "/registry/transactions/2020/02", nil))
	reply2 := buf2.Result()

	if reply2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for GET on monthly filter but instead got %v\n", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		var probeTrx expenses.Transactions
		if err := expenses.FromJson(body, &probeTrx); err != nil {
			t.Fatal(err)
		}

		if len(probeTrx) != len(dataTransactions) {
			t.Fatalf("Expected %d transaction for year 2020 month 02 since there's feed from examples but got: %v\n", len(dataTransactions), len(probeTrx))
		}
	}
}
