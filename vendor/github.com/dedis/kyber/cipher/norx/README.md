### Go Implementation of NORX

[NORX](https://norx.io) is a parallel and scalable authenticated encryption algorithm and was designed by:

  * [Jean-Philippe Aumasson](http://aumasson.jp)
  * [Philipp Jovanovic](http://cryptomaths.com)
  * [Samuel Neves](http://eden.dei.uc.pt/~sneves/)

This implementation currently supports only NORX64 in sequential mode.



####Installation
```
go get https://github.com/Daeinar/norx-go
```

####Usage
The following command installs norx-go and runs the test vectors from `test.go`:
```
go install && norx-go
```

####License
This software package is released under the BSD (3-Clause) license. See the file `LICENSE` for more details.
