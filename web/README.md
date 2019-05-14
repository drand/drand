# Drand Web App

## Requirements

- [hugo](https://gohugo.io)

## Testing

In order to test, we have to start a fake drand server to get the randomness from. To so such, fo to the folder `server_example` and run:
```
python3 script.py
```

The public key, previous and randomness fields are generated thanks to the `server_example/main.go` file.

## Production

The identity struct at line 97 of `layouts/index.html` should be modified accordingly to the address of the server to contact. If given Key is "" then it'll be fetched from the server as well.

In file `static/js/display.js` you can change how the randomness strings are printed (i.e., with/without a timestamp, ...)

The `static/congif/group.toml` file should be the one given to the participants.
(Note: if group.toml is uploaded for ex to /info/group.toml then code can be modified to fetch it too).

Then start the web server on localhost:1313 by running:

```
make
```
and deploy with
```
make deploy
```

## Other

Design by HTML5 UP under the [Creative Commons Attribution License 3.0](https://html5up.net/license)
