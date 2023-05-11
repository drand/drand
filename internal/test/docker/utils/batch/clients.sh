#bin/sh

NODES=5
BEACONS=20
PERIOD_STEP=5

for (( node = 0; node < 5; node++ )); do

  echo "-------------- NODE ${node} --------------"
  echo "------------HTTP Randomness on default------------"
  RAND=`docker exec drand_client /bin/sh -c "drand-client --url http://drand_${node}:8${node}81 --insecure"`
  echo "Value: ${HASH}"
  echo ""

  echo "------------GRPC Randomness on default----------"
  RAND=`docker exec drand_client /bin/sh -c "drand-client --grpc-connect drand_${node}:8${node}80 --insecure"`
  echo "Value: ${RAND}"
  echo ""

  echo "------------Hash on default------------"
  HASH=`docker exec drand_${node} /bin/sh -c "drand show chain-info --hash"`
  echo "Value: ${HASH}"
  echo ""

  echo "------------HTTP Randomness on default (chain ${HASH})------------"
  RAND=`docker exec drand_client /bin/sh -c "drand-client --url http://drand_${node}:8${node}81 --insecure --chain-hash ${HASH}"`
  echo "Value: ${RAND}"
  echo ""

  echo "------------GRPC Randomness on default (chain ${HASH})----------"
  RAND=`docker exec drand_client /bin/sh -c "drand-client --grpc-connect drand_${node}:8${node}80 --insecure --chain-hash ${HASH}"`
  echo "Value: ${RAND}"
  echo ""

  for (( beacon = 1; beacon <= BEACONS; beacon++ )); do
      PERIOD=$((${beacon}*PERIOD_STEP))

      echo "------------Hash on beacon_${PERIOD}s------------"
      HASH=`docker exec drand_${node} /bin/sh -c "drand show chain-info --id beacon_${PERIOD}s --hash"`
      echo "Value: ${HASH}"
      echo ""

      echo "------------HTTP - Hash - Randomness on beacon_${PERIOD}s (chain ${HASH})----------"
      RAND=`docker exec drand_client /bin/sh -c "drand-client --url http://drand_${node}:8${node}81 --insecure --chain-hash ${HASH}"`
      echo "Value: ${RAND}"
      echo ""

      echo "------------GRPC - Hash - Randomness on beacon_${PERIOD}s (chain ${HASH})----------"
      RAND=`docker exec drand_client /bin/sh -c "drand-client --grpc-connect drand_${node}:8${node}80 --insecure --chain-hash ${HASH}"`
      echo "Value: ${RAND}"
      echo ""

      echo "------------HTTP - Group - Randomness on beacon_${PERIOD}s (chain ${HASH})----------"
      RAND=`docker exec drand_client /bin/sh -c "drand-client --url http://drand_${node}:8${node}81 --insecure --group-conf /data/drand_${node}/.drand/multibeacon/beacon_${PERIOD}s/groups/drand_group.toml"`
      echo "Value: ${RAND}"
      echo ""

      echo "------------GRPC - Group - Randomness on beacon_${PERIOD}s (chain ${HASH})----------"
      RAND=`docker exec drand_client /bin/sh -c "drand-client --grpc-connect drand_${node}:8${node}80 --insecure --chain-hash ${HASH} --group-conf /data/drand_${node}/.drand/multibeacon/beacon_${PERIOD}s/groups/drand_group.toml"`
      echo "Value: ${RAND}"
      echo ""


  done

  echo ""
  echo ""
  echo ""
done
