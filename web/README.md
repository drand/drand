
# Drand Web App

This is the source code of `drand`'s website, used to smartly present the outputs of the beacon and its network configuration. It comes with additional functionalities such as:
- Possibility to navigate through the randomness history,
- Verification of the generated randomness against the distributed key, using [drandjs](https://github.com/PizzaWhisperer/drandjs),
- First contacted node is randomly picked from the latest [configuration file](https://github.com/dedis/drand/blob/master/deploy/latest/group.toml) hosted on Github,
- User can choose which node of the group is contacted to fetch the randomness from.

You can find a live example at [zerobyte.io](https://drand.zerobyte.io).

## Run it

#### Requirements
- [hugo](https://gohugo.io),
- local copy of the code, which can be downloaded with `go get -u github.com/dedis/drand` or `git clone https://github.com/dedis/drand`,
- an SSH setup compatible with https://gohugo.io/hosting-and-deployment/deployment-with-rsync/#install-ssh-key.

#### Build
Start the web server on localhost:1313 by running:
```
make
```

#### Deploy
There are two ways to deploy the website.
- Use `deploy.sh` script, which will ask for the user and host names compatible with your SSH setup, as well as the path on the server and the URL you want to deploy this website to. You can refer to the help menu to see how to use this script or more informations if you have some trouble identifying the parameters:
```
sh deploy.sh --help
```
.
- _(more advanced)_ Manually overwrite the `DEST` variable in the [makefile](https://github.com/dedis/drand/blob/web/web/Makefile#L2) with the path on your server and the `baseURL` in [config.toml](https://github.com/dedis/drand/blob/web/web/config.toml#L1) with your website's URL, then run:
```
make deploy
```

## Other

Design by HTML5 UP under the [Creative Commons Attribution License 3.0](https://html5up.net/license)
