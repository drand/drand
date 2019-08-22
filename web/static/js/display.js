const latestDiv = document.querySelector('#latest');
const roundDiv = document.querySelector('#round');
const verifyButton = document.querySelector('#verify');
const nodesListDiv = document.querySelector('#nodes');

window.verified = false;

//counter used to navigate through randomness indexes
var currRound = "0";
//interval id for bar progress
var idBar = -1;

/**
* displayRandomness is the main function which display
* the latest randomness and nodes when opening the page
**/
async function displayRandomness() {
  //when started, contact random node from group file
  if (window.identity == "") {
    findFirstNode();
    while (window.identity == "") {
      //wait for promise to find node (seriously there must be another way)
      await sleep(1);
    }
  }
  //start the progress bar
  move();
  //print randomness and update verfified status
  fetchAndVerify(window.identity, window.distkey)
  .then(function (fulfilled) {
    window.verified = true;
    printRound(fulfilled.randomness, fulfilled.previous, fulfilled.round, true);
  })
  .catch(function (error) {
    window.verified = false;
    printRound(error.randomness, error.previous, error.round, false);
  });
  //print servers on map
  //printNodesMap();
  //print server as list
  printNodesList();
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
function printRound(randomness, previous, round, verified) {
  if (round < currRound || round == undefined) {
    return
  }
  currRound = round;

  //print randomness as current
  var p = document.createElement("pre");
  digestMessage(randomness).then(digestValue => {
    var randomness = hexString(digestValue);
    var quarter = Math.ceil(randomness.length / 4);
    var s1 = randomness.slice(0, quarter);
    var s2 = randomness.slice(quarter, 2*quarter);
    var s3 = randomness.slice(2*quarter, 3*quarter);
    var s4 = randomness.slice(3*quarter);
    var randomness_4lines =  s1 + '\n' + s2 + "\n" + s3 + "\n" + s4;
    var textnode = document.createTextNode(randomness_4lines);
    p.appendChild(textnode);
    latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  });

  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; p.style.cursor = "pointer"};
  p.onmouseout = function() {p.style.textDecoration = "none";};
  if (window.identity.TLS){
    var reqURL = 'https://'+ window.identity.Address + '/api/public';
  } else {
    var reqURL = 'http://'+ window.identity.Address + '/api/public';
  }
  var jsonStr = '{"round":'+round+',"previous":"'+previous+ '","randomness":{"gid": 21,"point":"'+randomness+ '"}}';
  var modal = document.getElementById("myModal");
  p.onclick = function() {
    var modalcontent = document.getElementById("jsonHolder");
    modalcontent.innerHTML = 'Request URL: <strong>'+ reqURL + '</strong> <br> Raw JSON: <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    modal.style.display = "block";
  };
  window.onclick = function(event) {
    if (event.target == modal) {
      modal.style.display = "none";
    }
  }

  //index info
  var p2 = document.createElement("pre");
  var textnode2 = document.createTextNode(round);
  p2.appendChild(textnode2);
  roundDiv.replaceChild(p2, roundDiv.childNodes[0]);

  //refresh verify button
  refreshVerify();
}

/**
* printNodesMap prints interactive map of drand nodes
**/
function printNodesMap() {
  var nodes = [
    JSON.parse('{"addr": "drand.cothority.net:7003", "lat": 48.86, "lon": 2.3444}'),
    JSON.parse('{"addr": "drand.zerobyte.io:8888", "lat": 41.827637, "lon": 2.462732}'),
    JSON.parse('{"addr": "drand.nikkolasg.xyz:8888", "lat": 50.989125, "lon": 9.205674}'),
    JSON.parse('{"addr": "drand.lbarman.ch:443", "lat": 45.289125, "lon": 13.205674}'),
    JSON.parse('{"addr": "drand.kudelskisecurity.com:443", "lat": 39.789125, "lon": 9.205674}'),
    JSON.parse('{"addr": "drand.protocol.ai:8080", "lat": 44.667, "lon": -122.833}'),
    JSON.parse('{"addr": "random.uchile.cl:8080", "lat": -33.781682, "lon": -70.924195}'),
    JSON.parse('{"addr": "drand.cloudflare.com:443", "lat": 37.792032, "lon": -122.394613}')
  ];

  $(function () {
    $(".mapcontainer").mapael({
      //default settings
      map: {
        name: "world_countries",
        defaultArea: {
          attrs: {
            fill: "#d1d1d1"
            , stroke: "#d1d1d1"
          },
          attrsHover: {
            fill: "#d1d1d1"
            , stroke: "#d1d1d1"
          }
        },
        defaultPlot: {
          size:20,
          factor: 0.6,
          attrs: {
            fill:"#99D19A",
            stroke: "#fff"
          },
          eventHandlers: {
            click: function (e, id, mapElem, textElem, elemOptions) {
              window.identity = {Address: elemOptions.tooltip.content, TLS: true};
              refresh();
            }
          }
        }
      }
    });
    //for each node, look at status and show by adding to plot list if up
    for (let id in nodes) {
      var node = nodes[id];
      isUp(nodes[id].addr, true)
      .then((rand) => {
        var node = nodes[id];
        var updatedOptions = JSON.parse('{"newPlots": {"' + node.addr +'": {"latitude":"' + node.lat +'", "longitude":"' + node.lon + '", "tooltip": { "content":"' + node.addr +'"}}}}');
        $(".mapcontainer").trigger('update', updatedOptions);
      })
      .catch((err) => {console.log(nodes[id].addr + ' is down');});
    }
  });
}

/**
* printNodeList prints interactive list of drand nodes
**/
function printNodesList() {

  fetchGroup(window.identity).then(group => {
    nodesListDiv.innerHTML="";
    var i = 0;
    while (i < group.nodes.length) {
      let addr = group.nodes[i].address;
      let host = addr.split(":")[0];
      let port = addr.split(":")[1];
      let tls = group.nodes[i].TLS;

      let line = document.createElement("tr");
      let statusCol = document.createElement("td");
      isUp(addr, tls)
      .then((rand) => {
        statusCol.innerHTML = '<td> &nbsp;&nbsp;&nbsp; ✔️ </td>';
        statusCol.style.color= "transparent";
        statusCol.style.textShadow= "0 0 0 green";
      })
      .catch((err) => {statusCol.innerHTML = '<td> &nbsp;&nbsp;&nbsp; 🚫 </td>';});
      line.appendChild(statusCol);
      let addrCol = document.createElement("td");
      addrCol.innerHTML = '<td>' + host + '</td>';
      addrCol.onmouseover = function() { addrCol.style.textDecoration = "underline"; };
      addrCol.onmouseout = function() {addrCol.style.textDecoration = "none";};
      addrCol.onclick = function() {
        window.identity = {Address: addr, TLS: tls};
        refresh();
      };
      line.appendChild(addrCol);
      let portCol = document.createElement("td");
      portCol.innerHTML = '<td>' +port+'</td>';
      line.appendChild(portCol);
      let tlsCol = document.createElement("td");
      tlsCol.innerHTML = '<td> non tls </td>';
      if (tls) {
        tlsCol.innerHTML = '<td> tls </td>';
      }
      line.appendChild(tlsCol);
      let linkCol = document.createElement("td");
      linkCol.innerHTML = '<td><a title="https://' + addr + '/api/public" class="fa fa-external-link-alt" href="https://' + addr + '/api/public"></a></td>';
      linkCol.style.textAlign="center";
      line.appendChild(linkCol);
      if (addr == window.identity.Address) {
        line.style.fontWeight="bold";
      }
      nodesListDiv.appendChild(line);
      i = i + 1;
    }
  }).catch(error => console.error('Could not fetch group:', error))
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
  fetchAndVerifyRound(window.identity, window.distkey, round)
  .then(function (fulfilled) {
    printRound(fulfilled.randomness, fulfilled.previous, round, true);
  })
  .catch(function (error) {
    printRound(error.randomness, error.previous, round, false);
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
    fetchAndVerifyRound(window.identity, window.distkey, round)
    .then(function (fulfilled) {
      printRound(fulfilled.randomness, fulfilled.previous, round, true);
    })
    .catch(function (error) {
      printRound(error.randomness, error.previous, round, false);
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
* used to get upper bound for the prev/next navigation
**/
function getLatestIndex() {
  return new Promise(function(resolve, reject) {
    fetchPublic(window.identity).then((rand) => {resolve(rand.round);})
  });
}

/**
* digestMessage and hexString are used to hash the signature
**/
function digestMessage(message) {
  const encoder = new TextEncoder();
  const data = encoder.encode(message);
  return window.crypto.subtle.digest('SHA-512', data);
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

/**
* checkVerify checks if randomness was verified and changes button to ok or
* nope, refreshVerify puts button back to default
**/
function checkVerify(key) {
  if (window.verified) {
    verifyButton.innerHTML = '<a href="https://github.com/PizzaWhisperer/drandjs/" class="button alt icon small fa-check"> verified using drandjs, click here to discover our js library</a>';
  } else {
    verifyButton.innerHTML = '<a href="https://github.com/PizzaWhisperer/drandjs/" class="button alt icon solid small fa-times"> drandjs could not verify this randomness against the distributed key</a>';
  }
}

function refreshVerify() {
  verifyButton.innerHTML = '<a class="button alt solid small" onclick="checkVerify()">Verify</a>';
}

/**
* findFirstNode picks a first node to contact at random from the up servers
* starts by reading last configuration file from github repo, filters the
* addresses and tries until success to contact a server with tls from the pool
**/
function findFirstNode() {
  fetch('https://raw.githubusercontent.com/dedis/drand/master/deploy/latest/group.toml')
  .then(function(response) {
    response.text()
    .then((text) => {
      let addrList = Object.values(text.split('\n')).filter(str => str.includes("Address")).map(item => item.substring(13, item.length - 1));
      let rndId = Math.floor(Math.random() * addrList.length);
      isUp(addrList[rndId], true)
      .then((result) => {
        window.identity = {Address: addrList[rndId], TLS: true};
        return
      })
      .catch((err) => {
        findFirstNode();
      });
    });
  })
  .catch(err => {
    console.log("could not get the group from github");
  });
}

/**
* sleep makes the main thread wait ms milliseconds before continuing
**/
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
