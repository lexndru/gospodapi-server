# gospodapi
Gospodapi is a server-side application to build and maintain general ledgers. The name is a wordplay between "gospodărie" (ro) meaning household and "API" (en abbr).

# Known issues and limits of the implementation
- No pagination: this may cause problems to clients which cannot support long (as in time) bigger (as in size) tranfeser of data;
- No standard errors: although it's covered in tests, there may be unexpected use-cases for a client to encounter as a direct Golang error in plain text on a 5xx status code response;
- The "cache feature" is always on: there's no option to disable cache for reading/listing with GET verbs;
- Golang ioutil: some modules (e.g. registry, insights) use a memory intensive module which may cause DOS on some systems;
- Upserting transactions by UUID with detailed breakdown of the amount will cause data corruption;

# Todo
- [ ] Save process state while gracefullly shutting down with close() method
- [ ] Awake must check if another process is also running
- [ ] Add flag to `-share` database with other instances of the process on the same VM (prevent awake to exit on PID duplicate)
- [ ] Benchmark API request against PostgreSQL/MariaDB/MySQL/SQLite
- [x] Add ON UPDATE CASCADE for actors and labels name changes
- [x] Add FK constraint for self ref. (parent) column in labels

# License
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
