# Design Rationals

## Private key generation

In drand, it is assumed that all participating nodes know each other's public key and
addresses. This information comes from a publicly known "group.toml" file.
In order to create this group file, every node run the daemon using a keygen
command which generates and output a fresh public / private identity. Nodes
operator must copy the public part and insert that information into the globally
public group file.

## Distributed randomness

After the private key generation step, every node can run the daemon with the
private key, and group file as arguments. The daemon will open an port and wait
for further instructions. An initiator must add an additional flag to the daemon
to contact the other nodes in order to start the distributed key generation and
distributed randomness protocol.

After the DKG has been running correctly, the threshold BLS signature protocol
is started by the initiator automatically at a fixed interval.
