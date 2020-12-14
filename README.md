Gneto
========================================

Gneto is a personal proxy to make content from the [Gemini Protocol](https://gemini.circumlunar.space/) available over HTTP.

Start Gneto like:

```
$ gneto
```

…then point your web browser at [your new local Gemini proxy server](http://localhost:8065).

Run `gneto --help` to see all the command-line options.


Building Gneto
----------------------------------------

Gneto has no dependencies apart from the standard Go library.

```
$ git clone https://github.com/pgorman/gneto
$ cd gneto
$ go build
$ ./gneto
```

Copyright
----------------------------------------

Gneto copyright Paul Gorman 2020.

Licensed under the GPL. See LICENSE.txt for details.