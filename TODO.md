- dealing with node failures from the leader during DKG and during TBLS
- self signatures (BLS) on Public struct
- not infinite loop in Drand
- not test with files inside the repo
- make DKG handle responses before deals
- store config in /etc/drand 
        data (signatures) in /var/lib/drand
        keys in ~/.drand/
- catch sigkill & stuff -> use it in systemd after !
- more, much more unit tests
