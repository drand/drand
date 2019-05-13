# Drand Web App

## Requirements

- [hugo](https://gohugo.io)

## Testing

In order to test, we have to start a fake drand server to get the randomness from. To so such, run:
```
python3 server_example/script.py
```

The public key, previous and randomness fields are generated thanks to the `server_example/main.go` file.

## Production

When production ready, the folder `server_example` can be removed.

The identity struct at line 9 (or 48) of `static/js/display.js` should be modified accordingly to the address of the server to contact.

The `static/congif/group.toml` file should be the one given to the partipants.
(Note: if group.toml is uploaded for ex to /info/group.toml then code can be modified to fetch it too).

Then start the webserver on localhost:1313 by running:

```
make
```
and deploy with
```
make deploy
```

## Other

Design by HTML5 UP under the [Creative Commons Attribution License](https://html5up.net/license)
