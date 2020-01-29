# Demo example

Make sure you have docker and docker-compose installed

Then run
```bash
sudo ./run.sh
```
(sudo may not be required to run docker depending on your setup, try without
first!)

# Using

Fetch randomness from the first node by running:
```bash
curl $(sudo docker inspect --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' drand1):8080/api/public
```

You can change the `drand1` to `drand{1-5}` to fetch randomness from another
node.
Command first tries to get the IP address of the first container, and calls the
REST API on it.
