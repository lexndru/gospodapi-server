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
	"archive/zip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/lexndru/expenses"

	"gorm.io/gorm"
)

type uploader struct {
	Transactions expenses.Transactions
	Actors       expenses.Actors
	Labels       expenses.Labels
}

func (u *uploader) LockZip(zfp string) (exists bool, err error) {
	fsep := string(filepath.Separator)
	lock := filepath.Dir(zfp) + fsep + "." + filepath.Base(zfp)

	if _, err = os.Stat(lock); os.IsNotExist(err) {
		err = ioutil.WriteFile(lock, nil, 0644)
		exists = false
	} else {
		exists = true // file already existed?
	}

	return
}

func (u *uploader) FromZip(zfp string) (err error) {
	var zf *zip.ReadCloser
	if zf, err = zip.OpenReader(zfp); err != nil {
		return
	}

	defer zf.Close()

	var exists bool
	if exists, err = u.LockZip(zfp); exists {
		return // Perhaps this should belong somewhere else
	}

	unpack := func(f *zip.File) []byte {
		if fd, err := f.Open(); err != nil {
			panic(err)
		} else {
			defer fd.Close()
			if bytez, err := ioutil.ReadAll(fd); err != nil {
				panic(err)
			} else {
				return bytez
			}
		}
	}

	for _, file := range zf.File {
		if file.Name == "reg_transactions.json" {
			err = expenses.FromJson(unpack(file), &u.Transactions)
		} else if file.Name == "reg_actors.json" {
			err = expenses.FromJson(unpack(file), &u.Actors)
		} else if file.Name == "reg_labels.json" {
			err = expenses.FromJson(unpack(file), &u.Labels)
		} else {
			fmt.Printf("Unsupported file to unpack: %s\n", file.Name)
		}

		if err != nil {
			return
		}
	}

	return
}

func (u *uploader) Commit(ctx expenses.PushContext) {
	mustPush := func(ctx expenses.PushContext, r expenses.Registry) {
		if err := r.Push(ctx); err != nil {
			panic(err)
		}
	}

	mustPush(ctx, &u.Actors)
	mustPush(ctx, &u.Labels)
	mustPush(ctx, &u.Transactions)
}

func restore(db *gorm.DB, batch int, zipfile string) {
	u := uploader{}
	u.FromZip(zipfile)
	u.Commit(expenses.PushContext{Storage: db, BatchSize: batch})
}
