Gneto
========================================

Gneto is a personal proxy server to make content from the [Gemini Protocol](https://gemini.circumlunar.space/) available over HTTP.

[![Gneto screenshot](https://paulgorman.org/software/gneto/gneto-screenshot.png)](https://paulgorman.org/software/gneto/gneto-screenshot.png)


Features
----------------------------------------

- Gento makes Gemini content accessible on platforms that do not yet have mature Gemini clients.
- If you want a Gemini to HTTP proxy, Gneto improves your privacy by not replying on a proxy hosted by someone else.
- No JavaScript. Browse from Lynx if you want.
- Transient client certificates are supported.
- Customize Gneto's look with standard CSS. Example light and dark themes are provided.
- Gneto works well running on your workstation's loopback interface, a server on your home LAN, or (with a password enabled) on your public server.

Running Gneto From A Binary
----------------------------------------

Start Gneto like:

```
$ cd gneto/
$ ./gneto
```

…then point your web browser at [your new local Gemini proxy server](http://localhost:8065).

Run `gneto --help` to see all Gneto's command-line options.


Building Gneto
----------------------------------------

Gneto has no dependencies apart from the standard Go library.

```
$ git clone https://github.com/pgorman/gneto
$ cd gneto
$ go build
$ ./gneto
```


Limitations and Known Bugs
----------------------------------------

Limitations:

- Handling of sensitive input submission needs testing. Don't use it for super-secret stuff yet!
- Gneto only supports transient client certificates at this time. There's no way to have it present a persistent TLS client certificate to a Gemini server. This feature will likely be implemented in the near future.


Security Considerations
----------------------------------------

Gneto is designed as a single-user proxy, typically running on the loopback interface of the same machine running your web browser.

There are two security considerations:

1. Unless you set the environment variable `password`, Gneto operates as an open proxy. If you run Gneto on an IP address accessible to someone besides you, set a strong value for `password`.
2. If client certificates are turned on, Gneto maintains a single pool of client certificates. Therefore, everyone with access to Gneto presents the same identity to Gemini servers. This may be undesirable, even if you only share Gneto with other members of your household. If you share Gneto, set `--hours 0` to turn off transient client certificates.

If you must run a public, open proxy with Gneto, please set these options:

```
$ gneto --textonly --hours 0
```

If you run Gneto on your own public server, for your own private use, set the `password` environment variable, like:

```
$ password='myv3ry-Strongpassssword' gneto
```

FAQ
----------------------------------------

### How can Gneto run as a service?

On a Linux system running systemd, run Gneto as a user service by copying the `gneto.service` file to `$HOME/.config/systemd/user/`. The unit file assumes Gneto is installed in `$HOME/bin/gneto/`, so edit `gneto.service` if you've installed it elsewhere. Then, activate the unit:

```
$ systemctl --user daemon-reload
$ systemctl --user enable gneto
$ systemctl --user start gneto
$ systemctl --user status gneto
```

### How do I customize the way Gneto looks?

When running Gneto, the `--css` command-line option allows specifies a custom cascading style sheet, like:

```
$ gneto --css $HOME/.gneto/custom.css
```

A light style sheets is provided in `web/light.css`.

### How do I change the links shown on the home/start page.

Specify a local Gemini file, like:

```
$ gneto --home ~/myhomepage.gmi
```

### What command-line options does Gneto accept?

```
$ gneto --help
```

### Firefox gives a "connection timed out" error sometimes!

In Firefox's preferences, search for "proxy". Select "Auto-detect proxy settings for this network".


Copyright
----------------------------------------

Gneto copyright 2020 Paul Gorman.

Licensed under the GPL. See LICENSE.txt for details.
