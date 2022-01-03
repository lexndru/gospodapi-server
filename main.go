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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gorilla/mux"
	"github.com/lexndru/expenses"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const VOID = "void?"

var (
	VERSION string
	BUILD   string
	OSARCH  string
	DRIVER  string
	LICENSE string
)

var (
	driver     func(string) gorm.Dialector
	database   *gorm.DB
	multiplex  = mux.NewRouter()
	httpRouter = multiplex.PathPrefix("/v0").Subrouter()
)

type config struct {
	configName string
	address    string
	timeout    time.Duration
	backupFile zipBackup
	batchSize  int
}

type zipBackup struct {
	set   bool
	value string
}

func (zb *zipBackup) Set(s string) error {
	zb.value = s
	zb.set = true

	return nil
}

func (zb *zipBackup) String() string {
	return zb.value
}

var args = config{configName: ".gospodapi"}

type app struct {
	LastKnownProcessId   int
	LastSuccessfulStart  int64
	LastGracefulShutdown int64
	LastBackupRestored   string
	IsRegistryInstalled  bool

	BuildPlatform  string
	BuildNumber    string
	FullVersion    string
	DatabaseDriver string
}

func (a app) Save() {
	if LICENSE != VOID {
		if data, err := json.Marshal(a); err != nil {
			fmt.Printf("cannot save state of app because: %v", err)
		} else {
			ioutil.WriteFile(args.configName, data, 0644)
		}
	}
}

var gospodapi = app{}

func awake() {
	if LICENSE != VOID {
		data, _ := ioutil.ReadFile(args.configName)
		if len(data) > 1 {
			if err := json.Unmarshal(data, &gospodapi); err != nil {
				panic("cannot wake from corrupted file because of an error: " + err.Error())
			}
		}
	}
}

func init() {
	switch DRIVER {
	case "psql":
		driver = postgres.Open
	case "mysql":
		driver = mysql.Open
	case "sqlite":
		driver = sqlite.Open
	default:
		driver = func(s string) gorm.Dialector {
			fmt.Println("WARNING: this build is using temporary storage")
			return sqlite.Open("file::memory:?cache=shared")
		}
	}

	if DRIVER == "" {
		fmt.Println("WARNING: void driver fallbacks to in-memory database")
		DRIVER = "void"
	}

	if LICENSE == "" {
		fmt.Println("WARNING: you are running an unlicensed build")
		LICENSE = VOID
	}
}

func shell() {
	flag.StringVar(&args.address, "bind", "127.0.0.1:9121", "address to bind")
	flag.DurationVar(&args.timeout, "timeout", time.Second*60, "http i/o timeout")
	flag.IntVar(&args.batchSize, "batch", 1000, "batch size for database i/o")
	flag.Var(&args.backupFile, "restore", "optional backup to restore on boot")
	flag.Parse()
}

func main() {
	shell() // initialize command line arguments from shell
	awake() // various checks and constraints lookup, e.g. has registry been installed? has backup been restored?
	setup() // setup the connection to the main database pool and allocate it globally for other modules to use

	{ /* begin setup for registry module */
		if gospodapi.IsRegistryInstalled {
			fmt.Println("NOTICE: registry has been previously installed ...")
		} else {
			if err := expenses.Install(database); err != nil {
				panic(err)
			}

			gospodapi.IsRegistryInstalled = true
			gospodapi.Save()

			fmt.Printf("Succesfully installed registry module on %v database ...\n", DRIVER)
		}

		if LICENSE == "cloud" {
			database.Exec(`alter table labels add foreign key (parent_name) references labels(name) on update cascade`)
		}

		mod := &registry{database, args.batchSize}
		mod.Setup(httpRouter)
	} /* done with registry module */

	{ /* begin setup for journal module */
		mod := journal{database, args.batchSize}
		mod.Setup(httpRouter)
	} /* done with journal module */

	{ /* begin backup restore from zip file */
		if args.backupFile.set {
			if gospodapi.LastBackupRestored == args.backupFile.value {
				fmt.Printf("NOTICE: request to restore backup is ignored to avoid data overwrite\n")
			} else {
				restore(database, args.batchSize, args.backupFile.value)
				gospodapi.LastBackupRestored = args.backupFile.value
				gospodapi.Save()
				fmt.Printf("Succesfully restored backup from zip %v\n", args.backupFile.value)
			}
		}
	} /* done with backup restore */

	fmt.Printf(`Booting v%s_%s; %s; %s ...

                                   _              _
                                  | |            (_)
   __ _  ___   ___ _ __   ___   __| |  __ _ _ __  _
  / _' |/ _ \ / __| '_ \ / _ \ / _' | / _' | '_ \| |
 | (_| | (_) |\__ \ |_) | (_) | (_| || (_| | |_) | |
  \__, |\___(_)___/ .__/ \___/ \__,_(_)__,_| .__/|_|
   __/ |          | |                      | |
  |___/           |_|                      |_|


Copyright (c) 2021 Alexandru Catrina <alex@codeissues.net>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
`+"\n", VERSION, LICENSE, OSARCH, BUILD)

	gospodapi.FullVersion = VERSION
	gospodapi.BuildNumber = BUILD
	gospodapi.BuildPlatform = OSARCH
	gospodapi.DatabaseDriver = DRIVER
	gospodapi.LastKnownProcessId = os.Getpid()
	gospodapi.LastSuccessfulStart = time.Now().Unix()
	gospodapi.Save()

	start() // bootstrap the http server and start serving requests
	clean() // begin clean procedure on shutdown signal
}

func setup() {
	var err error
	if database, err = gorm.Open(driver(os.Getenv("DB_DSN")), &gorm.Config{}); err != nil {
		panic(err)
	}

	fmt.Printf("Connected to %s database ... \n", DRIVER)
}

func start() {
	addr := args.address
	tout := args.timeout

	server := &http.Server{
		Handler:      multiplex,
		Addr:         addr,
		WriteTimeout: tout,
		ReadTimeout:  tout,
		IdleTimeout:  tout,
	}

	defer server.Close()

	multiplex.HandleFunc("/", status) // register process healthcheck

	log.Printf("Ready to serve HTTP requests on %s (timeout %v)\n", addr, tout)
	log.Fatal(server.ListenAndServe())
}

func clean() {
	// close api
}

type introspection struct {
	Troubleshoot string
	MemoryHeap   string
	MemoryTalloc string
	MemoryOS     string
	MemoryFree   string
	MemoryLastGC uint64
}

var mb uint64 = 1024 * 1024

func status(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	self := introspection{
		MemoryHeap:   fmt.Sprintf(`%v MB`, m.Alloc/mb),
		MemoryTalloc: fmt.Sprintf(`%v MB`, m.TotalAlloc/mb),
		MemoryOS:     fmt.Sprintf(`%v MB`, m.Sys/mb),
		MemoryFree:   fmt.Sprintf(`%v MB`, m.Frees/mb),
		MemoryLastGC: m.LastGC,
	}

	if database != nil {
		if db, err := database.DB(); err != nil {
			self.Troubleshoot = fmt.Sprintf("database connection error: %s", err.Error())
		} else if err := db.Ping(); err != nil {
			self.Troubleshoot = fmt.Sprintf("database ping error: %s", err.Error())
		}
	} else {
		self.Troubleshoot = "database is missing"
	}

	selfJson, selfErr := json.Marshal(self)
	procJson, procErr := json.Marshal(gospodapi)

	if selfErr != nil || procErr != nil {
		out := fmt.Sprintf("proc.err=%s\nself.err=%s", procErr, selfErr)

		fmt.Fprint(w, out)
	}

	output := fmt.Sprintf(`[%s, %s]`, procJson, selfJson)

	fmt.Fprint(w, output)
}

// response helper struct used to respond back for 2xx, 4xx, 5xx
type Response struct {
	Writer http.ResponseWriter
}

func (r Response) Wrong(err error, req *http.Request) {
	r.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	r.Writer.Header().Set("X-Server", fmt.Sprintf("gospodapi v%s_%s; %s; %s", VERSION, LICENSE, OSARCH, BUILD))
	r.Writer.WriteHeader(http.StatusBadRequest)

	fmt.Fprint(r.Writer, err.Error())
	log.Printf(" %5s %-80s [400] %12v\n", req.Method, req.URL.Path, err)
}

func (r Response) Fault(err error, req *http.Request) {
	r.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	r.Writer.Header().Set("X-Server", fmt.Sprintf("gospodapi v%s_%s; %s; %s", VERSION, LICENSE, OSARCH, BUILD))
	r.Writer.WriteHeader(http.StatusInternalServerError)

	fmt.Fprint(r.Writer, err.Error()) // TODO: perhaps use json and return error code as int
	log.Printf(" %5s %-80s [500] %12v\n", req.Method, req.URL.Path, err)
}

func (r Response) Okay(output []byte, isCached bool, lap time.Duration, req *http.Request) {
	r.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.Writer.Header().Set("X-Cache", fmt.Sprintf("%v", isCached))
	r.Writer.Header().Set("X-Benchmark", fmt.Sprintf("%v", lap))
	r.Writer.Header().Set("X-Server", fmt.Sprintf("gospodapi v%s_%s; %s; %s", VERSION, LICENSE, OSARCH, BUILD))
	r.Writer.WriteHeader(http.StatusOK)

	var cacheInfo string
	if isCached {
		cacheInfo = "(from cache)"
	}

	fmt.Fprint(r.Writer, string(output))
	log.Printf(" %5s %-80s [200] %12v %s\n", req.Method, req.URL.Path, lap, cacheInfo)
}

func (r Response) OkayStream(output []byte, isCached bool, lap time.Duration, req *http.Request) {
	r.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	r.Writer.Header().Set("X-Benchmark", fmt.Sprintf("%v", lap))
	r.Writer.Header().Set("X-Server", fmt.Sprintf("gospodapi v%s_%s; %s; %s", VERSION, LICENSE, OSARCH, BUILD))
	r.Writer.WriteHeader(http.StatusOK)

	var cacheInfo string
	if isCached {
		cacheInfo = "(from cache)"
	}

	fmt.Fprint(r.Writer, string(output))
	log.Printf(" %5s %-80s [200] %12v %s\n", req.Method, req.URL.Path, lap, cacheInfo)
}
