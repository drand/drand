const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
const nodesDiv = document.querySelector('#nodes');
window.identity = "";
window.distkey = "";

var lastRound = "0";
var idBar = -1;

function display_randomness() {
  var identity = window.identity;
  var distkey = window.distkey;

  //start the progress bar
  move();
  //get timestamp
  var date = new Date();
  var timestamp = date.toString().substring(3);
  //print randomness
  fetchAndVerify(identity, distkey)
  .then(function (fulfilled) {
    print_round(fulfilled.randomness, fulfilled.previous, fulfilled.round, true, timestamp);
  })
  .catch(function (error) {
    print_round(error.randomness, error.previous, error.round, false, timestamp);
  });
  //print servers
  print_nodes(identity);
}

//handles the progress bar
function move() {
  var elem = document.getElementById("myBar");
  var width = 0;
  if (idBar != -1) {
    window.clearInterval(idBar);
  }
  idBar = setInterval(frame, 60);
  function frame() {
    if (width >= 100) {
      clearInterval(id);
    } else {
      width += 0.1;
      elem.style.width = width + '%';
    }
  }
}

//prints the current randomness and updates the history
function print_round(randomness, previous, round, verified, timestamp) {
  if (round <= lastRound || round == undefined) {
    return
  }
  lastRound = round;

  var middle = Math.ceil(randomness.length / 2);
  var s1 = randomness.slice(0, middle);
  var s2 = randomness.slice(middle);
  var randomness_2lines =  s1 + " \ " + s2;

  //print randomness as current
  var p = document.createElement("p");
  var p2 = document.createElement("p");
  var textnode = document.createTextNode(randomness_2lines);
  p.appendChild(textnode);
  latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  if (verified) {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " verified");
  } else {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " unverified");
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

  //append previous randomness to history
  var p3 = document.createElement("p");
  p3.style.fontSize = '13px';
  var round_minus_one = round - 1;
  var textnode3 = document.createTextNode('(' + round_minus_one + ') ' + previous);
  p3.appendChild(textnode3);
  historyDiv.insertBefore(p3, historyDiv.childNodes[0]);
  //if more than 15 remove last one
  if (historyDiv.childElementCount >= 10) {
    historyDiv.removeChild(historyDiv.lastChild);
  }
}

//prints interactive list of drand nodes
function print_nodes(identity) {
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
          window.clearInterval(id);
          display_randomness();
          window.setInterval(display_randomness, 60000);
        };
        line.appendChild(statusCol);
        line.appendChild(addrCol);
        nodesDiv.appendChild(line);
        i = i + 1;
      }
    }).catch(error => console.error('Could not fetch group:', error))
  }
}

//decides if node is reachable by trying to fetch randomness
function isUp(addr, tls) {
  return new Promise(function(resolve, reject) {
    fetchPublic({Address: addr, TLS: tls})
    .then((rand) => {resolve(true);})
    .catch((error) => {reject(false);});
  });
}
