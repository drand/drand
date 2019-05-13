var fetchAndVerify = function(identity) {
  return new Promise(function(resolve, reject) {
    var pub_key = 0; var previous = 0; var randomness = 0; var round = 0; var err = 0;

    fetchKey(identity).then(key => {
      pub_key = key.key.point
      fetchPublic(identity).then(rand => {
        previous = rand.previous
        randomness = rand.randomness.point
        round = rand.round.toString();
        if (verify_drand(previous, randomness, round, pub_key)) {
          resolve([randomness, round]);
        } else {
          reject([randomness, round]);
        }
      })
    })
  });
}

var fetchAndVerifyWithKey = function(identity) {
  return new Promise(function(resolve, reject) {
    var pub_key = 0; var previous = 0; var randomness = 0; var round = 0; var err = 0;
    fetchPublic(identity).then(rand => {
      pub_key = identity.Key
      previous = rand.previous
      randomness = rand.randomness.point
      round = rand.round.toString();
      if (verify_drand(previous, randomness, round, pub_key)) {
        resolve([randomness, round]);
      } else {
        reject([randomness, round]);
      }
    })
  });
}

window.fetchAndVerify = fetchAndVerify
window.fetchAndVerifyWithKey = fetchAndVerifyWithKey
