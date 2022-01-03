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
	"os"
	"path/filepath"
	"testing"

	"github.com/lexndru/expenses"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const T_ZREG = "examples/reg.zip"

func TestUploadFromZipFile(t *testing.T) {
	if _, err := os.Stat(T_ZREG); os.IsNotExist(err) {
		t.Skip("no zip file to test")
	}

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := expenses.Install(db); err != nil {
		t.Fatal(err)
	}

	var legacy = &uploader{}

	if err := legacy.FromZip(T_ZREG); err != nil {
		t.Fatal(err)
	}

	if len(legacy.Actors) == 0 {
		t.Fatal("Expected actors after unpack, got nothing")
	}

	if len(legacy.Labels) == 0 {
		t.Fatal("Expected labels after unpack, got nothing")
	}

	if len(legacy.Transactions) == 0 {
		t.Fatal("Expected transactions after unpack, got nothing")
	}

	legacy.Commit(expenses.PushContext{Storage: db, BatchSize: 1000}) // panics if doesn't work

	lockzf := filepath.Dir(T_ZREG) + "/." + filepath.Base(T_ZREG)
	if err := os.Remove(lockzf); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreWithWrongInputs(t *testing.T) {
	if _, err := os.Stat(T_ZREG); os.IsNotExist(err) {
		t.Skip("no zip file to restore")
	}

	defer func() {
		lockzf := filepath.Dir(T_ZREG) + "/." + filepath.Base(T_ZREG)
		if err := os.Remove(lockzf); err != nil {
			t.Fatal(err)
		}

		if err := recover(); err == nil {
			t.Fatal("Expected restore to fail")
		}
	}()

	restore(nil, 0, T_ZREG)

	t.Fatal("Expected restore to fail and not reach this line")
}
