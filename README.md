# gophx-db-test

A test tool for the Golang Phoenix database meetup in October 2018.

## Test Your Database

To run a validation of your database implementation, first start your
application so it's listening on the required port. Then `go get` this test
repo, move into it's directory, build the binary, and run it:

```sh
go get github.com/alexsasharegan/gophx-db-test
cd $GOPATH/src/github.com/alexsasharegan/gophx-db-test
go build main.go

./main
```

I've set timeouts in case test commands don't receive their expected input, so
any deadlocks should fail after ~3 seconds. There's one test case that your
implementation may not pass. It pertains to spaces in values. I'm curious who
else supports this.

## Benchmark Your Database

Results will vary depending on the hardware the client & benchmark is run. There
are flags to configure the benchmark characteristics:

- `-bench` (bool): Required to run the benchmark.
- `-time` (integer): Sets the duration of the benchmark in seconds _(defaults to
  10 seconds)_.
- `-procs` (integer): Sets the number of clients that will interact with the DB
  during the benchmark _(defaults to number of CPUs)_.
- `-v`: Outputs extra logging (does not log in the benchmark loop).

You should ensure your database passes the main test before running a benchmark
(minus the one test hinted above). In any case, the benchmark will also force a
failure if it does not complete 1 second after the benchmark duration.

To run a benchmark of your database implementation, first start your application
so it's listening on the required port. Then `go get` this test repo, move into
it's directory, build the binary, and run it:

```sh
go get github.com/alexsasharegan/gophx-db-test
cd $GOPATH/src/github.com/alexsasharegan/gophx-db-test
go build main.go

./main -bench

# or with optional configuration flags
./main -bench -procs=8 -time=5
```
