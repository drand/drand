# drand v2

We want to change the following:
 - changing transport layer to allow HTTP(s) 
 - better organization / modularization of the code
 - introducing the private randomness functionality
 - think of a versioning mechanism that would allow 
    + using different suite / keys in the future 
 - think of an updating functionality

# Transport layer

For some security and business reason, we have been asked to make drand work
with regular port 80 and 443 (HTTP(s)). There are two questions:
    1. how the communication endpoints should be like by switching to this
       transaction-based transport ?
    2. should we allow different transport implementation ? is it worth the
      time/effort ?

Here are some potential answers to both of these questions:

## System endpoints using HTTP

There are different ways we can build this: 

### REST API 

Regular REST API over HTTP so the URL logically defines the operations
available.  This API could use JSON or protobuf encoding, as it would bring minimal changes
to the current drand encoding functions.
However, REST is more ressource-oriented and fits very well for CRUD operations.
That is not the case for drand and not in its foreseeable future.

### JSON-RPC 

Also wildly known & easily understandable. Although using JSON forces us to not
have any types. Also we have to implement a JSON decoder which works with our
Marshalling kyber interface.

### gRPC

Akin to JSON-RPC but using protobuf so it inherits all the advantages about
protobuf serialization, speed, extensibility, versionning and so on. 
Allows clients to use the same APIs as backend services. It's also supported by
Google so it's highly developed and used already by big players (Netflix,
CoreOS, etc).
We also need a separate implementation of the protobuf v3 code that works with
our Marshalling kyber interface.

Personal conclusion:

I'm highly tempted to go for the gRPC solution.

## Generic transport implementation

Should we allow for having a generic transport implementation for this current
refactoring ? The goal would be to be able to switch from http transport to
regular tcp connection, potentially with Noise or stg.

I have concerns about trying to go for a full generic transport layer for drand:
    + Is it fulfilling a real concern ? (such as the one Cloudflare had about
      non HTTP solutions). Drand operators won't likely change their transport
      out of HTTP, if they need another port, it would be already possible to do
      so. I can't see any reasons why someone would say "I don't want HTTP in my
      network" ... 
    + Related to the first point, should we spent time / resources on this ? I
      see only a very little gain for quite some amount of additional work.
    + If we allow for many different types of transport, we potentially forces
      drand operators to have two ports: one for public internet-wide API for
      clients that wishes to retrieve randomness, and one for the internal
      system to run the randomness beacon... It seems harder to justify and also
      potentially bring much more complexity to the codebase (won't justify why
      but it seems pretty clear). Having HTTP only solutions makes it nmuch
      nicer to have this unified endpoint / address with different
      functionalities in it. It also makes the process of keeping an updated
      node lists much easier: one node one address.

Of course, the benefits would be that it could potentially be integrated within
the DEDIS onet framework. I have also some reserves about that:
    + The onet framework uses a very strongly static way of handling identities
      at the moment. ServerIdentity uses the key defined statically,Some of its
      transport does not even provide real authentication, but merely an
      exchange of identities. I would prefer the network layer to just relay
      messages without taking care of the real authentication at the protocol
      level. This hurts onet now for having different services / different
      endpoints using different suites and so on.
    + There's no versionning capabilities at the moment.
    + If I were to change onet, I'd have to go through a much more strict
      and hindering process than just drand only. Indeed, onet is used by many
      projects here, and the master branch has just been marked as non API
      breaking for some months. Theses changes are to be brought by the team of
      software engineers and I'll be happy to share my experience about the
      current changes of drand.
    + For interoperability, it becomes much more harder to be compatible with
      onet than with a regular gRPC endpoint (where the .proto file is enough to
      be compatible).

I have been heavily involved in the design and implementation of the onet
network layer, but after a few years I see some drawbacks and weaknesses that
I'd like to avoid in drand. In any case, I'll be happy to talk more about these
to the team of SE.

Personal conclusion:

I'm very tempted to just go with a full HTTP solution but still using interfaces
to be able to have some flexibility in the future (and for easier testing).


# Modularization of the code

Currently most files are at the top level. I wish to bring modularization to
this new version so we would have something like:

+ / -> top level containing readme, main entrypoint for cli and packages
information etc
+ /net/ -> network layer 
+ /dkg/ -> dkg protocol
+ /beacon/ -> randomness beacon protocol
+ /commands/ -> commands for cli & to the daemon
+ ...

This is only a very rough draft and the concrete list will become much more
apparent during the implementation itself.

# Introducing private randomness functionality

This is to be discussed a bit more in detail with Vassilev Apostol, our contact
at NIST that is very interested in the project and had this idea.
Basically, a client could request any number of drand nodes for "regular"
randomness. The client would send a public key in the request as well, so the
drand node can reply with an encrypted version of the randomness. There's some
details to fill in with Apostol obviously but that would be the idea in order to
get a fully functional "network randomness protocol" akin to NTP for the time.

# Versioning mechanism

If we go for the protobuf gRPC approach, then versioning is automatically
handled by protobuf, since new fields will be understood by newer versions but
older clients will still be able to decode messages.

# Updating functionality

In order to be able to deploy new functionalities and correct bugs at a fast
pace for the early stage (where it's mostly trying out, see if it makes sense
and works), we'd like to be able to provide a limited auto updating functionality.
For example, one way to do it, is to make the node check on the github
repository if a new tag / version has been updated since the current running
version, and if so, download it and runs it. This check should not be done
automatically but upon a request from a predefined hardcoded key or if some
conditions are met on the github tag / version (for example a custom field /
file indicating that automatically updating is OK and there's no API breaking
changes).
This has to be discussed more in details of course, but it could be nice to have
that since drand operators in the early stages are going to be very remote and
there's high chances of having a high rate of bugfixes / functionalities added
in the beginning

