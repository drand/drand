#bin/sh

NODES=5
BEACONS=20
PERIOD_STEP=5

nohup docker exec -u drand drand_0 /bin/sh -c "drand share --leader --nodes 5 --threshold 4 --period 2s" &
sleep 1
nohup docker exec -u drand drand_2 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable" &
sleep 1
nohup docker exec -u drand drand_1 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable" &
sleep 1
nohup docker exec -u drand drand_3 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable" &
sleep 1
nohup docker exec -u drand drand_4 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable" &
sleep 1

for (( beacon = 1; beacon <= BEACONS; beacon++ )); do

  PERIOD=$((${beacon}*PERIOD_STEP))
  docker exec -u drand drand_0 /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_0:8080"
  docker exec -u drand drand_1 /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_1:8180"
  docker exec -u drand drand_2 /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_2:8280"
  docker exec -u drand drand_3 /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_3:8380"
  docker exec -u drand drand_4 /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_4:8480"


  nohup docker exec -u drand drand_0 /bin/sh -c "drand share --leader --nodes 5 --threshold 4 --period ${PERIOD}s --id beacon_${PERIOD}s --scheme pedersen-bls-unchained" &
  sleep 1
  nohup docker exec -u drand drand_2 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable --id beacon_${PERIOD}s" &
  sleep 1
  nohup docker exec -u drand drand_1 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable --id beacon_${PERIOD}s" &
  sleep 1
  nohup docker exec -u drand drand_3 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable --id beacon_${PERIOD}s" &
  sleep 1
  nohup docker exec -u drand drand_4 /bin/sh -c "drand share --connect drand_0:8080 --tls-disable --id beacon_${PERIOD}s" &
  sleep 1

done