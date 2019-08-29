# Drand on docker

The main readme for the drand project is [here](../README.md). This readme describes how to run a *production* version of `drand` based on `docker-compose`. 

**Note:** this is meant as a production setup; it notably involves generating TLS certificates for your public-facing server. If you simply want a local demo of drand, run `make demo` in the root folder instead.

## Prerequisites for this guide

a VPS with the following software setup:

1. `docker >= 17.12`
2. `docker-compose >= 1.18`
3. `go >= 1.12`
4. `certbot`, a TLS-capable reverse-proxy, or any other way to get TLS certificates (see section "Setting up TLS")

## First steps

Copy/send the `deploy-example` folder on your server, then open a shell in it.
You may place this directory where you want, e.g. `~/deploy-example`. Its name is irrelevant too, should you want to change it (just don't call it `~/.drand` which is used for the config files).

At this point, your current working directory should look like this:

```bash
$ pwd
../drand/deploy-example
------------------------------------------------------------
$ tree
.
├── data
│   ├── tls_certificates
│   └── tls_keypair
├── docker-compose.yml
└── README_docker.md
```

Also make sure `data` is owned by your user, and have rights `740`:

```bash
chmod -R 740 data
```

## Setting up TLS

To be secure, drand needs authenticated channels to talk to other drand nodes. This can be currently done in two ways:

1. via the TLS module within drand; in that case, you need to give TLS certificates to drand itself.
2. via a reverse-proxy in front of drand; in that case, drand itself is unaware of TLS, and your reverse proxy is handling TLS itself.

### First option: use TLS within drand

One way to get a TLS certificate is through (LetsEncrypt)[https://certbot.eff.org/lets-encrypt/debianjessie-other] and their command-line tool `certbot`.

Use `certbox` to generate TLS certificates. The command will depend on your setup, but typically can be `sudo certbot certonly --standalone`.

Once done, `certbot` put the files in `/etc/letsencrypt/live`. We are interested in `/etc/letsencrypt/live/YOURSERVER/certX.pem` and `/etc/letsencrypt/live/YOURSERVER/privkeyX.pem`.

Copy those two files into `data/tls_keypair`, renaming them as `cert.pem` and `privkey.pem`:

```bash
cp /etc/letsencrypt/live/YOURSERVER/certX.pem data/cert.pem
cp /etc/letsencrypt/live/YOURSERVER/privX.pem data/priv.pem
```

The TLS setup is done.

### Second option: disable TLS in drand

Note: **only** do this if you intend to setup TLS with your reverse proxy. If you don't use TLS at all, there's no point in doing this setup, it won't be secure ! If you're just trying to run an insecure demo, run `make demo` in the root folder of this repository instead of following this guide.

In this case, replace the following line in the `docker-compose.yml` file:

```
    command: --verbose 2 start --listen 0.0.0.0:8080 --cert-dir "/root/.drand/tls_certificates" --tls-cert "/root/.drand/tls_keypair/cert.pem" --tls-key "/root/.drand/tls_keypair/key.pem"
```

by:
```
    command: --verbose 2 -tls-disable start --listen 0.0.0.0:8080
```

This guide will continue focusing on drand; jump to the end of this guide to configure the reverse proxy.

## Generate drand keys

Now, let's generate keys for drand:

```bash
go get -u github.com/dedis/drand
drand generate-keypair <address>
```

Where `<address>` is for instance `drand.yourserver.com:8080` or `yourserver.com:8080`. If you'll be using a reverse-proxy, make sure you enter the public-facing port.

This generates keys in `~/.drand/keys/`. Let's move them into `data`:

```bash
mv ~/.drand/key data
```

## Docker-compose setup

Drand can now be started as follows:

```bash
docker-compose up -d
```

To check what is happening, access the docker-compose logs via
```bash
docker-compose logs
```

## Distributed Key Generation

If you did the setup above, you have a container running the drand deamon, loaded with your keys. It still misses two things:

1. the `group.toml` file corresponding to other participants. For this, you have to exchange keys manually, e.g., via email.
2. running the DKG protocol to bootstrap drand.

Fortunately, with our docker-compose volumes, it's now very easy to add things into the running container. Just add your `group.toml` into the root of the `data` folder (**NOT** in the `data/groups/` folder; this one is manually managed by drand, don't touch it).

Then, open a CLI into your running docker.

First find its id on the host:

```bash
$ docker ps
697e4766f8b2        drand_drand             "drand --verbose 2 s…"   11 minutes ago      Up 9 minutes
```

The id of the container is `697e4766f8b2`. Enter it by running:

```bash
docker exec -it 697e4766f8b2 /bin/sh
```

Then, you're inside the container; tell drand to run the DKG like so:

```bash
drand share /root/.drand/group.toml
```

**Notice the full path** `/root/.drand/group.toml` and not `group.toml` nor `./group.toml`

At this point, once *everybody* in the group.toml has run the same command (at the same time), the randomness generation starts. Well done! Simply let it run, there's nothing else to do.

## Other topics

### Updating drand

To update drand, simply shut it the container

```bash
docker-compose down
```

and to fully rebuild it, you need to first clean the already-used layers (for some reason Docker is confused and thinks nothing has changed, and it keeps rebuilding the old version). Caution! this delete *all* your *unused* containers and networks; it's typically fine, but just be aware of it.

```bash
docker system prune -a
```

Then rebuild and restart it

```bash
docker-compose up --build -d
```

### Reset the docker state (without losing the keys)

This part is if you need to reset drand's internal state without loosing the keys. 

#### Method 1: using `drand clean`

First, try using this method. If that doesn't work, use the method below.

Find drand's container id on the host, and enter it:

```bash
$ docker ps
697e4766f8b2        drand_drand             "drand --verbose 2 s…"   11 minutes ago      Up 9 minutes

$ docker exec -it 697e4766f8b2 /bin/sh
```

Then simply call:

```bash
drand reset
```

Exit the container with `CTRL-C`. Then, on the host, I advise you to restart the container (to make sure the drand deamon has a clean restart and can reload its cleaned config):

```bash
docker-compose down
docker-compose up --build -d
```

#### Method 2: doing a reset manually

The method above relies on `drand clean`, which could theoretically fail. If you want a manual hard-reset, start by killing the container:

```bash
docker-compose down
```

Delete what you want to reset in `data`: typically, you absolutely want to **keep** `data/keys`, **especially** if you shared those keys to create a `group.toml` with other people. For instance if the DKG failed, remove `data/db` and `data/groups`. Notice that if you added the `group.toml` into the root of `data` as suggested, it should still be there (don't delete it unless you want to change the group).

Then, rebuild the image from scratch:

```bash
docker system prune -a
docker-compose up --build -d
```

Check that things are running with

```
docker-compose logs
```

You're now back to the step "Distributed Key Generation" of this guide.

### Docker behind reverse proxy setup

Typically, the TLS part of my VPS is managed by a single reverse proxy, which then proxies multiple services running locally with docker.

There is one subtletly: you need to forward _both_ GRPC (used by drand "core") and web traffic (for the web interface). To forward GRPC, you need to have nginx `1.13.10` or above, it's a fairly recent addition.

Then, you need to forward differently traffic to `/` and to `/api/`. Here's an example configuration for `nginx`:

```
server {
  server_name drand.lbarman.ch;
  listen 443 ssl http2;
  ssl_protocols   SSLv3 TLSv1 TLSv1.1 TLSv1.2;
  ssl_ciphers   HIGH:!aNULL:!MD5;
  location / {
    grpc_pass grpc://localhost:1234;
  }
  location /api/ {
    proxy_pass http://localhost:1234; 
    proxy_set_header Host $host;
  }
  
  ssl_certificate /etc/letsencrypt/live/.../fullchain.pem; # managed by Certbot
  ssl_certificate_key /etc/letsencrypt/live/.../privkey.pem; # managed by Certbot
}
```

*Note:* to others, you'll still be using TLS (handled by your reverse proxy), so make sure you generate your drand-keys using an https address, and the flag `TLS=true`.