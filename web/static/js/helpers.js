//fetches the randomness
function fetchPublic(identity) {
  var fullPath = identity.Address + "/api/public";
  if (identity.TLS == false) {
    fullPath = "http://" + fullPath;
  } else  {
    fullPath = "https://" + fullPath;
  }
  return fetch(fullPath).then(resp => Promise.resolve(resp.json()));
}

//fetches the public key
function fetchKey(identity) {
  var fullPath = identity.Address + "/api/info/distkey";
  if (identity.TLS == false) {
    fullPath = "http://" + fullPath;
  } else  {
    fullPath = "https://" + fullPath;
  }
  return fetch(fullPath).then(resp => Promise.resolve(resp.json()));
}

//fetches the group file
function fetchGroup(identity) {
  var fullPath = identity.Address + "/api/info/group";
  if (identity.TLS == false) {
    fullPath = "http://" + fullPath;
  } else  {
    fullPath = "https://" + fullPath;
  }
  return fetch(fullPath).then(resp => Promise.resolve(resp.json()));
}

//converts hex string to bytes array
function hexToBytes(hex) {
    for (var bytes = [], c = 0; c < hex.length; c += 2)
    bytes.push(parseInt(hex.substr(c, 2), 16));
    return bytes;
}

//converts int to bytes array
function intToBytes(int) {
    var bytes = [];
    var i = 8;
    do {
    bytes[--i] = int & (255);
    int = int>>8;
    } while ( i )
    return bytes;
}

//from msg and round to what was signed
function message(msg, round) {
  var b_msg = hexToBytes(msg);
  var b_round = intToBytes(round);
  return b_round.concat(b_msg);
}

//formats the received strings and verifies signature
function verify_drand(previous, randomness, round, pub_key) {
  var msg = message(previous, round);
  var p = new kyber.pairing.point.BN256G2Point();
  p.unmarshalBinary(hexToBytes(pub_key));
  var sig = hexToBytes(randomness);
  return kyber.sign.bls.verify(msg, p, sig);
}
