# Drand Web App

## Requirements

- [hugo](https://gohugo.io)

## Production

The identity struct at line 112 of `layouts/index.html` should be modified accordingly to the address of the drand server to contact. If given Key is "" then it'll be fetched from the server as well.

In file `static/js/display.js` you can change how the randomness strings are printed (i.e., with/without a timestamp, list of runing nodes...).

## Test Server

If one does not know a drand node to contact, they can start a fake drand server to get the randomness from. To so such run:
```
python3 api/script.py
```

The public key, previous and randomness fields that you can find in the `api` folder are generated with the `api/main.go` file. The group file is taken from a former drand network.

## Features
- Latest randomness with round, timestamp and verified check,
- Click on the latest randomness to see who was contacted and the associated raw JSON,
- Stack of the 10 previous rounds of randomness,
- List of running nodes, and possibility to click on one to contact it for the randomness.

## Deploy

Start the web server on localhost:1313 by running:

```
make
```
and deploy with
```
make deploy
```

## Other

Design by HTML5 UP under the [Creative Commons Attribution License 3.0](https://html5up.net/license)
