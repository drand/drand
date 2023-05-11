#bin/sh

NODES=5
BEACONS=20
PERIOD_STEP=5
LEADER_NODE=3
THRESHOLD=4


for (( beacon = 0; beacon <= BEACONS; beacon++ )); do

  PERIOD=$((${beacon}*PERIOD_STEP))

  for (( node = 0; node < NODES; node++ )); do

    if [ $beacon -eq 0 ]; then
      if [ $node -eq 0 ]; then
        nohup docker exec -u drand drand_${LEADER_NODE} /bin/sh -c "drand share --leader --nodes ${NODES} --threshold ${THRESHOLD} --period 2s" &
        sleep 1
      fi

      if [ $node -ne $LEADER_NODE ]; then
        nohup docker exec -u drand drand_${node} /bin/sh -c "drand share --connect drand_${LEADER_NODE}:8${LEADER_NODE}80 --tls-disable" &
        sleep 1
      fi

      continue
    fi

    if [ $node -eq 0 ]; then
      docker exec -u drand drand_${LEADER_NODE} /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_${LEADER_NODE}:8${LEADER_NODE}80"
      nohup docker exec -u drand drand_${LEADER_NODE} /bin/sh -c "drand share --leader --nodes ${NODES} --threshold ${THRESHOLD} --period ${PERIOD}s --id beacon_${PERIOD}s --scheme pedersen-bls-unchained" &
      sleep 1
    fi

    if [ $node -ne $LEADER_NODE ]; then
      docker exec -u drand drand_${node} /bin/sh -c "drand generate-keypair --tls-disable --id beacon_${PERIOD}s drand_${node}:8${node}80"
      nohup docker exec -u drand drand_${node} /bin/sh -c "drand share --connect drand_${LEADER_NODE}:8${LEADER_NODE}80 --tls-disable --id beacon_${PERIOD}s" &
      sleep 1
    fi

  done
done
