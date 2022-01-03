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
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/lexndru/expenses"

	"gorm.io/gorm"
)

type registry struct {
	dbInstance  *gorm.DB
	dbBatchSize int
}

func (r registry) Setup(router *mux.Router) {
	router.HandleFunc("/registry/transactions", r.writeJsonTransactions).Methods(http.MethodPost)
	router.HandleFunc("/registry/transactions", r.readJsonTransactions).Methods(http.MethodGet)
	router.HandleFunc("/registry/transactions/{year:[0-9]{4}}/{month:[0-9]{2}}", r.readJsonMonthlyTransactions).Methods(http.MethodGet)

	router.HandleFunc("/registry/labels", r.writeJsonLabels).Methods(http.MethodPost)
	router.HandleFunc("/registry/labels", r.readJsonLabels).Methods(http.MethodGet)

	router.HandleFunc("/registry/actors", r.writeJsonActors).Methods(http.MethodPost)
	router.HandleFunc("/registry/actors", r.readJsonActors).Methods(http.MethodGet)
}

var registryRoutesCache = make(map[string][]byte)

func (r registry) readJsonActors(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PullContext{Storage: r.dbInstance, Limit: r.dbBatchSize}

	_resolvePullRequest(&expenses.Actors{}, ctx, wr, rq)
}

func (r registry) writeJsonActors(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PushContext{Storage: r.dbInstance, BatchSize: r.dbBatchSize}

	_resolvePushRequest(&expenses.Actors{}, ctx, wr, rq)
}

func (r registry) readJsonLabels(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PullContext{Storage: r.dbInstance, Limit: r.dbBatchSize}

	_resolvePullRequest(&expenses.Labels{}, ctx, wr, rq)
}

func (r registry) writeJsonLabels(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PushContext{Storage: r.dbInstance, BatchSize: r.dbBatchSize}

	_resolvePushRequest(&expenses.Labels{}, ctx, wr, rq)
}

func (r registry) readJsonTransactions(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PullContext{Storage: r.dbInstance, Limit: r.dbBatchSize}

	_resolvePullRequest(&expenses.Transactions{}, ctx, wr, rq)
}

func (r registry) readJsonMonthlyTransactions(wr http.ResponseWriter, rq *http.Request) {
	params := mux.Vars(rq)

	year, _ := strconv.Atoi(params["year"])
	month, _ := strconv.Atoi(params["month"])

	period := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	t0, t1 := period, period.AddDate(0, 1, -1)

	ctx := expenses.PullContext{
		Storage: r.dbInstance.Where("date between ? and ?", t0, t1),
		Limit:   r.dbBatchSize,
	}

	_resolvePullRequest(&expenses.Transactions{}, ctx, wr, rq)
}

func (r registry) writeJsonTransactions(wr http.ResponseWriter, rq *http.Request) {
	ctx := expenses.PushContext{Storage: r.dbInstance, BatchSize: r.dbBatchSize}

	_resolvePushRequest(&expenses.Transactions{}, ctx, wr, rq)
	// transactions push request(s) can create not only transactions (with details)
	// but also new labels and new actors if necessary; for this reason it's safer
	// to just recreate the entire cache table
	registryRoutesCache = make(map[string][]byte)
}

func _resolvePullRequest(reg expenses.Registry, ctx expenses.PullContext, wr http.ResponseWriter, rq *http.Request) {
	startTime := time.Now()
	response := Response{wr}

	if cached, ok := registryRoutesCache[rq.URL.Path]; ok {
		response.Okay(cached, true, time.Since(startTime), rq)
		return // no need to continue
	}

	out, err := expenses.NewPullRequest(reg, ctx)

	if err != nil {
		response.Fault(err, rq)
	} else {
		response.Okay(out, false, time.Since(startTime), rq)
		registryRoutesCache[rq.URL.Path] = out
	}
}

func _resolvePushRequest(reg expenses.Registry, ctx expenses.PushContext, wr http.ResponseWriter, rq *http.Request) {
	startTime := time.Now()
	response := Response{wr}

	var payload []byte
	payload, err := ioutil.ReadAll(rq.Body)
	if err != nil { // TODO: avoid ioutil because of memory issues?
		response.Wrong(err, rq)
		return // wrong payload, don't continue
	} else {
		if err = expenses.FromJson(payload, reg); err != nil {
			response.Wrong(err, rq)
			return // wrong payload, can't continue
		}
	}

	out, err := expenses.NewPushRequest(reg, ctx)

	if err != nil {
		response.Fault(err, rq)
	} else {
		response.Okay(out, false, time.Since(startTime), rq)
		delete(registryRoutesCache, rq.URL.Path)
	}
}
