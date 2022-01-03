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
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/lexndru/expenses"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	journalDBInstance = sqlite.Open("file::memory:")
	journalHttpRouter *mux.Router
)

func init() {
	if db, err := gorm.Open(journalDBInstance, &gorm.Config{}); err != nil {
		panic(err)
	} else {
		journalHttpRouter = mux.NewRouter()

		expenses.Install(db)

		reg := registry{db, 10}
		reg.Setup(journalHttpRouter) // must register

		mod := journal{db, 10}
		mod.Setup(journalHttpRouter)
	}

	var dataTransactions expenses.Transactions
	jsonTransactions, _ := ioutil.ReadFile("examples/test_transactions.json")
	if err := expenses.FromJson(jsonTransactions, &dataTransactions); err != nil {
		panic(err)
	}

	log.SetOutput(ioutil.Discard)

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/registry/transactions", bytes.NewReader(jsonTransactions)))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		panic("cannot write transactions to registry")
	}

	// head request to compute model and cache both results and records
	journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("HEAD", "/journal/test-signature", nil))
}

func TestStreamSignedTransactionRecords(t *testing.T) {
	newline := []byte("\n")

	// NOTE: last 36 chars are for the assoc. UUID
	expected := []string{
		"actor.#6-xxxx-xxxx-xxxx-xxxxxxxxxxxx,actor.#1-xxxx-xxxx-xxxx-xxxxxxxxxxxx,Label #3,1581724800,1240000,",
		"Actor #1,Actor #2,Label #1.2,1580947200,-930,",
		"Actor #1,Actor #5,Label #1.1,1580947200,-1500,",
		"Actor #1,Actor #2,Label #1.1,1580860800,-3822,",
		"Actor #1,Actor #2,Label #1.2,1580860800,-2410,",
	}

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/journal/test-signature/download", nil))
	reply := buf.Result()

	if reply.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK but got %v", reply.StatusCode)
	}

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		lines := bytes.Split(body, newline)
		for i, line := range lines {
			if len(line) == 0 {
				break
			}

			if ln := line[0 : len(line)-36]; expected[i] != string(ln) {
				t.Fatalf("Expected %s but got %s", expected[i], line)
			}
		}
	}
}

func TestTryTransactionLabelsPrediction(t *testing.T) {
	// NOTE: at the moment models are build on read
	// TODO: change this because pagination won't be able to cache anymore
	// journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/journal/test-signature", nil))

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/journal/test-signature", bytes.NewReader([]byte(`[]`))))
	reply := buf.Result()

	if out, err := io.ReadAll(reply.Body); err != nil {
		t.Fatalf("Expected empty JSON array but got an error: %s", err)
	} else if string(out) != `[]` {
		t.Fatalf("Expected empty JSON array but something else: %s", out)
	}
}

func TestPredictiveModel(t *testing.T) {
	mockupJournalRecords := `[
		{
			"parent": "e1bd9319-e4f2-47cc-98b4-764d83d755dc",
			"date": "2020-02-16T00:00:00.000Z",
			"amount": -5000,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "538aefe5-b568-44f8-849b-5c1e09a1955f",
			"date": "2020-02-21T00:00:00.000Z",
			"amount": -4000,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "996971f1-4fb7-4617-8d00-a3a0cf88de7f",
			"date": "2020-02-25T00:00:00.000Z",
			"amount": -3000,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "7e33e52d-50f7-475b-8e24-94456a0c48b0",
			"date": "2020-06-25T00:00:00.000Z",
			"amount": -5899,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "04a887e9-c71d-43b4-88ff-1e5304a86a31",
			"date": "2020-09-03T00:00:00.000Z",
			"amount": -5149,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "79455d48-657b-45a1-91e2-93980b164667",
			"date": "2020-12-15T00:00:00.000Z",
			"amount": -4899,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "2a1019cb-b80a-4110-ba52-68c7f261751f",
			"date": "2021-02-05T00:00:00.000Z",
			"amount": -10299,
			"label": "Catering",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "f12742c0-1139-42c4-aa0c-2a3fed09cce7",
			"date": "2019-12-06T00:00:00.000Z",
			"amount": -5688,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "3560655a-421a-4b6d-b1a2-61637393d3ca",
			"date": "2019-12-21T00:00:00.000Z",
			"amount": -3017,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "19637fe1-8cf1-47c0-a05b-7e96c77f72f8",
			"date": "2020-01-31T00:00:00.000Z",
			"amount": -6541,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "a2a6018f-58e4-47fe-a6b9-019b354357c5",
			"date": "2020-02-15T00:00:00.000Z",
			"amount": -1197,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "2d947c20-f8ec-49c5-9d38-a068beaa3131",
			"date": "2020-02-18T00:00:00.000Z",
			"amount": -9563,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "b838f807-7fb9-4974-82b3-bc051379e973",
			"date": "2020-03-07T00:00:00.000Z",
			"amount": -4645,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "30a78d1e-35e6-4131-91be-6f060583796d",
			"date": "2020-07-28T00:00:00.000Z",
			"amount": -29400,
			"label": "Articole sănătate și fitness",
			"sender": "Alexandru",
			"receiver": "(livrator)"
		},
		{
			"parent": "a2a6018f-58e4-47fe-a6b9-019b354357c5",
			"date": "2020-03-09T00:00:00.000Z",
			"amount": -16197,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(supermarket)"
		},
		{
			"parent": "2d947c20-f8ec-49c5-9d38-a068beaa3131",
			"date": "2020-02-18T00:00:00.000Z",
			"amount": -19563,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(supermarket)"
		},
		{
			"parent": "b838f807-7fb9-4974-82b3-bc051379e973",
			"date": "2020-03-07T00:00:00.000Z",
			"amount": -14645,
			"label": "Alimente",
			"sender": "Alexandru",
			"receiver": "(supermarket)"
		}
	]`

	var records []record
	if err := json.Unmarshal([]byte(mockupJournalRecords), &records); err != nil {
		t.Fatalf("Incorrect mockup structure: %s", err)
	}

	model := _compute(records)

	if len(model) != 2 {
		t.Fatalf("Expected 2 entry in model but got %d", len(model))
	}

	for key, values := range model {
		if key == "Alexandru | (livrator)" {
			for _, features := range values {
				if features.Category != "Catering" {
					t.Fatalf("Expected features category to be Catering but got %s", features.Category)
				}
				if features.Polarity[0] != 0 {
					t.Fatalf("Expected polarity (incoming) to be zero but got %v", features.Polarity[0])
				}
				if features.Polarity[1] == 0 {
					t.Fatalf("Expected polarity (outgoing) to be greather than zero but got %v", features.Polarity[1])
				}
				if len(features.Amounts) == 0 {
					t.Fatalf("Expected amounts stack to be greather than zero but got %v", len(features.Amounts))
				}
				if len(features.Months) == 0 {
					t.Fatalf("Expected months stack to be greather than zero but got %v", len(features.Months))
				}
				if len(features.Weekdays) == 0 {
					t.Fatalf("Expected weekdays stack to be greather than zero but got %v", len(features.Weekdays))
				}
				if len(features.Days) == 0 {
					t.Fatalf("Expected days stack to be greather than zero but got %v", len(features.Days))
				}
			}
		}

		if key == "Alexandru | (supermarket)" {
			for _, features := range values {
				if features.Category != "Alimente" {
					t.Fatalf("Expected features category to be Alimente but got %s", features.Category)
				}
			}
		}
	}
}

func TestModelTendency(t *testing.T) {
	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/journal/test-signature", nil))
	reply := buf.Result()

	if out, err := io.ReadAll(reply.Body); err != nil {
		t.Fatalf("Expected JSON dump of model but got an error: %s", err)
	} else {
		var model tendency
		if err := json.Unmarshal(out, &model); err != nil {
			t.Fatalf("Incorrect unmarshall of model: %s", err)
		}

		if len(model) != 2 {
			t.Fatalf("Expected 2 keys as part of model but got %d", len(model))
		}

		if features, ok := model["sender=Actor #1 receiver=Actor #2"]; !ok {
			t.Fatal("Expected key with sender Actor #1 and receiver Actor #2 after study")
		} else {
			if len(features) != 2 {
				t.Fatalf("Expected 2 features but got %d", len(features))
			}

			var firstFeature = features[0]
			if firstFeature.Category != "Label #1.2" {
				t.Fatalf("Expected first feature to have category Label #1.2 but got %s", firstFeature.Category)
			}

			if firstFeature.Polarity[1] != len(firstFeature.Amounts) {
				t.Fatalf("Expected polarity to match size of amounts but got polarity %d and amounts %d", firstFeature.Polarity[1], len(firstFeature.Amounts))
			}

			// TODO: how can I cover this better?
		}
	}

	payload := bytes.NewReader([]byte(`[
		{
			"sender": "Actor #1",
			"receiver": "Actor #2",
			"amount": -1200,
			"date": "2021-04-02T00:00:00Z",
			"parent": "xxx"
		}
	]`))

	buf2 := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf2, httptest.NewRequest("POST", "/journal/test-signature", payload))
	reply2 := buf2.Result()

	if out, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatalf("Expected JSON dump of model but got an error: %s", err)
	} else {
		var outcome []statement
		if err := json.Unmarshal(out, &outcome); err != nil {
			t.Fatalf("Incorrect unmarshall of solutions: %s", err)
		}

		if len(outcome) != 1 {
			t.Fatalf("Expected 1 outcome, but got %d", len(outcome))
		}

		if len(outcome[0].Calculated) != 1 {
			t.Fatalf("Expected 1 prediction for label, but got %d", len(outcome[0].Calculated))
		}

		for name, points := range outcome[0].Calculated {
			if name != "Label #1.2" {
				t.Fatalf("Unexpected label prediction: %s", name)
			}

			if points[0] != 1 {
				t.Fatalf("Unexpected points calculated: %v", points)
			}

			// break
		}
	}
}

func TestSimilarTransactionsWithNothing(t *testing.T) {
	payload := bytes.NewReader([]byte(`[]`))

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/journal/test-signature", payload))
	reply := buf.Result()

	if out, err := io.ReadAll(reply.Body); err != nil {
		t.Fatalf("Expected empty JSON array but got an error: %s", err)
	} else if string(out) != `[]` {
		t.Fatalf("Expected empty JSON array but got something else: %s", out)
	}
}

func TestSimilarTransactionsWithPayload(t *testing.T) {
	payload := bytes.NewReader([]byte(`[
		{
			"date": "2020-02-06T00:00:00Z",
			"amount": -1500,
			"label": "",
			"sender": "Actor #1",
			"receiver": "Actor #5",
			"parent": "xxx"
		}
	]`))

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("POST", "/journal/test-signature", payload))
	reply := buf.Result()

	if out, err := io.ReadAll(reply.Body); err != nil {
		t.Fatalf("Expected empty JSON array but got an error: %s", err)
	} else {
		var results []statement
		if err := json.Unmarshal(out, &results); err != nil {
			t.Fatalf("Expected JSON array with comparison results but got an error: %s", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected one comparison to be found, but got nothing")
		}

		if len(results[0].Similarity) > 0 {
			if results[0].Similarity[0].Grade != 3 {
				t.Fatalf("Expected grade 3 similarity because of same sender and receiver but got %v", results[0].Similarity[0].Grade)
			}
		} else {
			t.Fatal("Expected one similarity to be found, but got none")
		}
	}
}

func TestTransactionIntegrityOnDetailsConflict(t *testing.T) {
	payload := bytes.NewReader([]byte(`[
		{
			"uuid": "5fc3b398-5b17-4ee3-a464-82af2c1b2ef9",
			"date": "2020-12-06T00:00:00Z",
			"amount": -3000,
			"label": "?",
			"sender": "Actor #1",
			"receiver": "Actor #5",
			"signature": "xxx",
			"details": [
				{
					"amount": 1000,
					"label": "Apă"
				},
				{
					"amount": 2000,
					"label": "Hrană pentru animale"
				}
			]
		}
	]`))

	journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/registry/transactions", payload))
	journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("HEAD", "/journal/xxx", nil))

	buf := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/journal/xxx/download", nil))
	reply := buf.Result()

	if body, err := io.ReadAll(reply.Body); err != nil {
		t.Fatal(err)
	} else {
		lines := strings.Split(strings.Trim(string(body), "\n"), "\n")
		if len(lines) != 3 { // 2 details + 1 record from loaded samples
			t.Fatalf("Expected three records to download but got %d instead", len(lines))
		}
	}

	payload2 := bytes.NewReader([]byte(`[
		{
			"uuid": "5fc3b398-5b17-4ee3-a464-82af2c1b2ef9",
			"date": "2020-12-06T00:00:00Z",
			"amount": -3000,
			"label": "?",
			"sender": "Actor #1",
			"receiver": "Actor #5",
			"signature": "xxx",
			"details": [
				{
					"amount": 1000,
					"label": "Apă"
				},
				{
					"amount": 1000,
					"label": "Hrană pentru căine"
				},
				{
					"amount": 1000,
					"label": "Hrană pentru pisică"
				}
			]
		}
	]`))

	journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/registry/transactions", payload2))
	journalHttpRouter.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("HEAD", "/journal/xxx", nil))

	buf2 := httptest.NewRecorder()
	journalHttpRouter.ServeHTTP(buf, httptest.NewRequest("GET", "/journal/xxx/download", nil))
	reply2 := buf2.Result()

	if body, err := io.ReadAll(reply2.Body); err != nil {
		t.Fatal(err)
	} else {
		lines := strings.Split(strings.Trim(string(body), "\n"), "\n")
		if len(lines) != 1 { // the only one record from loaded samples
			t.Fatalf("Expected corrupted transaction to be discarded and get one record but got %d instead", len(lines))
		}
	}
}
