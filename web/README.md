
# Drand Web App

This is the source code of `drand`'s' website, used to smartly present the outputs of the beacon and its network configuration. It comes with additional functionalities such as:
- Possibility to navigate through the randomness history,
- Verification of the generated randomness against the distributed key, using [drandjs](https://github.com/PizzaWhisperer/drandjs),
- First contacted node is randomly picked from the latest [configuration file](https://github.com/dedis/drand/blob/master/deploy/latest/group.toml) hosted on Github,
- User can choose which node of the group is contacted to fetch the randomness from.

You can find a running example at XXX.

## XXX

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
- Use the `deploy.sh` script, which will ask for USER, HOST, DIR and URL. You can refer to the help menu to guide you or if you need help with the parameters:
```
sh deploy.sh --help
```
.
- _(more advanced)_ Manually replace the `DEST` in the `makefile` with the path on your server and the `baseURL` with your website's URL in `config.toml`, then run:
```
make deploy
```

## Other

Design by HTML5 UP under the [Creative Commons Attribution License 3.0](https://html5up.net/license)
