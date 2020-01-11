const latestDiv = document.querySelector('#latest');
const roundDiv = document.querySelector('#round');
const verifyButton = document.querySelector('#verify');
const nodesListDiv = document.querySelector('#nodes');
var locationMap = new Map();

window.verified = false;

//counter used to navigate through randomness indexes
var currRound = "0";
//interval id for bar progress
var idBar = -1;

const fetchCommand = "fetchVerify";
window.worker.addEventListener('message', function(e) {
    var data = e.data;
    switch (data.cmd) {
        case fetchCommand:
            console.log("display.js: received fetch Command",data);
            window.verified = data.verified;
            printRound(data.randomness, data.previous, data.round, data.signature, data.verified);
    }
  }, false);

/**
* displayRandomness is the main function which display
* the latest randomness and nodes when opening the page
* the first contacted node is picked at random from group file
**/
async function displayRandomness() {
  if (window.identity == "") {
    findFirstNode();
    while (window.identity == "") {
      await sleep(1);
    }
  }
  startProgressBar();
  //print randomness and update verfified status
  window.worker.postMessage({
      cmd: fetchCommand, 
      identity: window.identity,
      distkey: window.distkey,
      round: drandjs.latestRound,
  });

  /*drandjs.fetchAndVerify(window.identity, window.distkey, drandjs.latestRound)*/
  //.then(function (fulfilled) {
      //console.log("fullfilled: ",fulfilled);
    //window.verified = true;
    //printRound(fulfilled.randomness, fulfilled.previous, fulfilled.round, "0", true);
  //})
  //.catch(function (error) {
      //console.log("NOT fullfilled: ",error);
    //window.verified = false;
    //printRound(error.randomness, error.previous, error.round, "0", false);
  /*});*/
  printNodesList();
}

/**
* startProgressBar handles the progress bar
**/
function startProgressBar() {
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
function printRound(randomness, previous, round, signature, verified) {
  if (round <= currRound || round == undefined || randomness == undefined || previous == undefined || signature == undefined) {
    return
  }
  currRound = round;

  //print randomness as current
  var p = document.createElement("pre");
  var quarter = Math.ceil(randomness.length/2);
  var s1 = randomness.slice(0, quarter);
  var s2 = randomness.slice(quarter, 2*quarter);
  var randomness_4lines =  s1 + '\n' + s2;
  var textnode = document.createTextNode(randomness_4lines);
  p.appendChild(textnode);
  latestDiv.replaceChild(p, latestDiv.childNodes[0]);

  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; p.style.cursor = "pointer"};
  p.onmouseout = function() {p.style.textDecoration = "none";};
  var jsonStr = '{"round":'+round+',"previous":"'+previous+'","signature":"'+signature+'","randomness":"'+randomness+'"}';
  var modal = document.getElementById("myModal");
  p.onclick = function() {
    if (window.identity.TLS){
      var reqURL = 'https://'+ window.identity.Address + '/api/public';
    } else {
      var reqURL = 'http://'+ window.identity.Address + '/api/public';
    }
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
* printNodeList prints interactive list of drand nodes
**/
function printNodesList() {
  drandjs.fetchGroup(window.identity).then(group => {
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
        console.log(addr," is  up");
      })
      .catch((err) => {
          statusCol.innerHTML = '<td> &nbsp;&nbsp;&nbsp; 🚫 </td>';
          console.log(addr," is NOT up",err);
      });
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

      var loc = locationMap.get(host);
      if (loc == undefined) { //did not fill map loc yet
        function handleResponse(json) {
          locationMap.set(host, json.country_code2);
          refresh();
        }
        getLoc(host, handleResponse);
      }
      loc = locationMap.get(host);
      if (loc == undefined) {
        loc = " ";
      }
      let countryCol = document.createElement("td");
      countryCol.innerHTML = '<td>' + loc + '</td>';
      line.appendChild(countryCol);

      let linkCol = document.createElement("td");
      linkCol.innerHTML = '<td><a title="https://' + addr + '/api/public" href="https://' + addr + '/api/public"><i class="fas fa-external-link-alt"></i></a></td>';
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
    drandjs.fetchLatest({Address: addr, TLS: tls})
    .then((r) => { console.log(addr," is up"); return Promise.resolve(r); })
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
  window.worker.postMessage({
      cmd: fetchCommand, 
      identity: window.identity,
      distkey: window.distkey,
      round: round,
  });
  /*drandjs.fetchAndVerify(window.identity, window.distkey, round)*/
  //.then(function (fulfilled) {
    //printRound(fulfilled.randomness, fulfilled.previous, round, "0", true);
  //})
  //.catch(function (error) {
    //printRound(error.randomness, error.previous, round, "0", false);
  /*});*/
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
    window.worker.postMessage({
      cmd: fetchCommand, 
      identity: window.identity,
      distkey: window.distkey,
      round: round,
  });

/*    drandjs.fetchAndVerify(window.identity, window.distkey, round)*/
    //.then(function (fulfilled) {
      //printRound(fulfilled.randomness, fulfilled.previous, round, "0", true);
    //})
    //.catch(function (error) {
      //printRound(error.randomness, error.previous, round, "0", false);
    /*});*/
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
    drandjs.fetchLatest(window.identity).then((rand) => {resolve(rand.round);})
  });
}

/**
* checkVerify checks if randomness was verified and changes button to ok or
* nope, refreshVerify puts button back to default
**/
function checkVerify(key) {
  if (window.verified) {
    verifyButton.innerHTML = '<a class="button alt icon solid small"> <i class="fas fa-check"></i> &nbsp; verified using drandjs </a>';
  } else {
    verifyButton.innerHTML = '<a class="button alt icon solid small"> <i class="fas fa-times"></i> &nbsp; drandjs could not verify this randomness against the distributed key</a>';
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
    alert("could not get the group from github, reload the page");
  });
}

/**
* sleep makes the main thread wait ms milliseconds before continuing
**/
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
* getLoc communicates with dns-js.com and geoIP.com APIs
**/
function getLoc(domain, callback) {
  var xhr = new XMLHttpRequest();
  URL = "https://www.dns-js.com/api.aspx";
  xhr.open("POST", URL);
  xhr.onreadystatechange = function () {
    if (this.readyState === XMLHttpRequest.DONE && this.status === 200) {
      let data = JSON.parse(xhr.response);
      var ip = data[0].Address;
      setIPAddressParameter(ip);
      setExcludesParameter("ip");
      setFieldsParameter("country_code2");
      getGeolocation(callback, "ca50c203abfa45a39fe376f3ba9d0a3f");
    }
  }
  xhr.send(JSON.stringify({Action: "Query", Domain: domain,Type: 1}));
}
