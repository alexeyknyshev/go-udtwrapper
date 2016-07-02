# go-udtwrapper

go-udtwrapper is a cgo wrapper around the main C++ UDT implementation.

This repository is the fork of the original
[getlantern/go-udtwrapper](https://github.com/getlantern/go-udtwrapper).
Several other forks are merged together here. Mainly it is based on
[jbenet/go-udtwrapper](https://github.com/jbenet/go-udtwrapper), mixed up with
[fffw/go-udtwrapper](https://github.com/fffw/go-udtwrapper) and
[Syncbak-Git/go-udtwrapper](https://github.com/Syncbak-Git/go-udtwrapper).
Original authors have been preserved for all imported commits, though some
commits were modified due to rebase and some commits were omitted.

## Usage

- Godoc: https://godoc.org/github.com/Lupus/go-udtwrapper/udt

## Tools:

- [udtcat](udtcat/) - netcat using the udt pkg
- [benchmark](benchmark/) - benchmark for comparison of tcp and udt streams

## Try:

```sh
(cd udtcat; go build; ./test_simple.sh)
```
