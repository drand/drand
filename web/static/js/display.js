const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
const nodesDiv = document.querySelector('#nodes');
window.identity = "";
window.distkey = "";

//counter used to navigate through randomness indexes
var currRound = "0";
var idBar = -1;

/**
* displayRandomness is the main function which display
* the latest randomness and nodes when opening the page
**/
function displayRandomness() {
  var identity = window.identity;
  var distkey = window.distkey;

  //start the progress bar
  move();
  //get readable timestamp
  var date = new Date();
  var timestamp = date.toString().substring(3, 34);
  //print randomness
  fetchAndVerify(identity, distkey)
  .then(function (fulfilled) {
    printRound(fulfilled.randomness, fulfilled.previous, fulfilled.round, true, timestamp);
  })
  .catch(function (error) {
    printRound(error.randomness, error.previous, error.round, false, timestamp);
  });
  //print servers
  printNodes(identity);
}

/**
* move handles the progress bar
**/
function move() {
  var elem = document.getElementById("myBar");
  var width = 0;
  if (idBar != -1) {
    window.clearInterval(idBar);
  }
  idBar = setInterval(frame, 60);
  function frame() {
    if (width >= 100) {
      clearInterval(idBar);
    } else {
      width += 0.1;
      elem.style.width = width + '%';
    }
  }
}

/**
* printRound formats and prints the given randomness with interactions
**/
function printRound(randomness, previous, round, verified, timestamp) {
  if (round <= currRound || round == undefined) {
    return
  }
  currRound = round;

  //print randomness as current
  var p = document.createElement("p");
  var p2 = document.createElement("p");
  digestMessage(randomness).then(digestValue => {
    var textnode = document.createTextNode(hexString(digestValue));
    p.appendChild(textnode);
    latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  });
  if (verified) {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " & verified");
  } else {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " & unverified");
  }
  p2.appendChild(textnode2);
  latestDiv.replaceChild(p2, latestDiv.childNodes[1]);

  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; };
  p.onmouseout = function() {p.style.textDecoration = "none";};
  var jsonStr = '{"round":'+round+',"previous":"'+previous+ '","randomness":{"gid": 21,"point":"'+randomness+ '"}}';
  var modal = document.getElementById("myModal");
  p.onclick = function() {
    var modalcontent = document.getElementById("jsonHolder");
    if (identity.TLS){
      modalcontent.innerHTML = 'Randomness fetched from https://'+ identity.Address + '/api/public:\n <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    } else {
      modalcontent.innerHTML = 'Randomness fetched from http://'+ identity.Address + '/api/public:\n <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    }
    modal.style.display = "block";
  };
  window.onclick = function(event) {
    if (event.target == modal) {
      modal.style.display = "none";
    }
  }
}

/**
* printNodes prints interactive list of drand nodes
**/
function printNodes(identity) {
  //only prints once
  if (nodesDiv.childElementCount == 0) {
    fetchGroup(identity).then(group => {
      var i = 0;
      while (i < group.nodes.length) {
        let addr = group.nodes[i].address;
        let tls = group.nodes[i].TLS;

        let line = document.createElement("tr");
        let statusCol = document.createElement("td");
        isUp(addr, tls)
        .then((rand) => {statusCol.innerHTML = '<td> ✅ </td>';})
        .catch((err) => {statusCol.innerHTML = '<td> 🚫 </td>';});
        let addrCol = document.createElement("td");
        addrCol.innerHTML = '<td>' + addr + '</td>';
        addrCol.onmouseover = function() { addrCol.style.textDecoration = "underline"; };
        addrCol.onmouseout = function() {addrCol.style.textDecoration = "none";};
        addrCol.onclick = function() {
          window.identity = {Address: addr, TLS: tls};
          refresh();
        };
        line.appendChild(statusCol);
        line.appendChild(addrCol);
        nodesDiv.appendChild(line);
        i = i + 1;
      }
    }).catch(error => console.error('Could not fetch group:', error))
  }
}

/**
* isUp decides if node is reachable by trying to fetch randomness
**/
function isUp(addr, tls) {
  return new Promise(function(resolve, reject) {
    fetchPublic({Address: addr, TLS: tls})
    .then((rand) => {resolve(true);})
    .catch((error) => {reject(false);});
  });
}

/**
* goToPrev navigates to previous randomness output
**/
function goToPrev() {
  if (currRound == 0) {
    return
  }
  currRound -= 2;
  round = currRound + 1;
  //stop the 60s chrono and progress bar
  window.clearInterval(id);
  window.clearInterval(idBar);
  var elem = document.getElementById("myBar");
  elem.style.width = 0 + '%';
  //print previous rand
  var identity = window.identity;
  var distkey = window.distkey;
  fetchAndVerifyRound(identity, distkey, round)
  .then(function (fulfilled) {
    printRound(fulfilled.randomness, fulfilled.previous, round, true, " _ ");
  })
  .catch(function (error) {
    printRound(error.randomness, error.previous, round, false, " _ ");
  });
}

/**
* goToNext navigates to next randomness output
**/
function goToNext() {
  getLatestIndex().then((latestRound) => {
    if (currRound == latestRound) {
      //we cannot go further
      return
    }
    if (currRound + 1 == latestRound) {
      //sync with latest randomness
      refresh();
      return
    }
    //update index
    round = currRound + 1;
    //stop the 60s chrono and progress bar
    window.clearInterval(id);
    window.clearInterval(idBar);
    var elem = document.getElementById("myBar");
    elem.style.width = 0 + '%';
    //print next rand
    var identity = window.identity;
    var distkey = window.distkey;
    fetchAndVerifyRound(identity, distkey, round)
    .then(function (fulfilled) {
      printRound(fulfilled.randomness, fulfilled.previous, round, true, "?");
    })
    .catch(function (error) {
      printRound(error.randomness, error.previous, round, false, "?");
    });
  });
}

/**
* refresh goes back to latest output
**/
function refresh() {
  window.clearInterval(id);
  displayRandomness();
  window.setInterval(displayRandomness, 60000);
}

/**
* getLatestIndex returns the index of the latest randomness
**/
function getLatestIndex() {
  return new Promise(function(resolve, reject) {
    var identity = window.identity;
    fetchPublic(identity).then((rand) => {resolve(rand.round);})
  });
}

function digestMessage(message) {
  const encoder = new TextEncoder();
  const data = encoder.encode(message);
  return window.crypto.subtle.digest('SHA-256', data);
}

function hexString(buffer) {
  const byteArray = new Uint8Array(buffer);

  const hexCodes = [...byteArray].map(value => {
    const hexCode = value.toString(16);
    const paddedHexCode = hexCode.padStart(2, '0');
    return paddedHexCode;
  });

  return hexCodes.join('');
}
