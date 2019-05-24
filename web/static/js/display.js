const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
const nodesDiv = document.querySelector('#nodes');
window.identity = "";
window.distkey = "";

var lastRound = "0";

function display_randomness() {
  var identity = window.identity;
  var distkey = window.distkey;

  move();
  var date = new Date();
  var timestamp = date.toString().substring(3);
  fetchAndVerify(identity, distkey)
    .then(function (fulfilled) {
      randomness = fulfilled[0];
      previous = fulfilled[1];
      round = fulfilled[2];
      print_round(randomness, previous, round, true, timestamp);
    })
    .catch(function (error) {
      randomness = error[0];
      previous = error[1];
      round = error[2];
      print_round(randomness, previous, round, false, timestamp);
    });
  print_nodes(identity);
}

function move() {
  var elem = document.getElementById("myBar");
  var width = 0;
  var id = setInterval(frame, 60);
  function frame() {
    if (width >= 100) {
      clearInterval(id);
    } else {
      width += 0.1;
      elem.style.width = width + '%';
    }
  }
}

function print_round(randomness, previous, round, verified, timestamp) {
  if (round == lastRound || round == undefined) {
    return
  }
  lastRound = round;

  var middle = Math.ceil(randomness.length / 2);
  var s1 = randomness.slice(0, middle);
  var s2 = randomness.slice(middle);
  var randomness_2lines =  s1 + " \ " + s2;

  //print randomness as current
  var p = document.createElement("p");
  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; };
  p.onmouseout = function() {p.style.textDecoration = "none";};
  var jsonStr = '{"round":'+round+',"previous":"'+previous+ '","randomness":{"gid": 21,"point":"'+randomness+ '"}}';
  p.onclick = function() {
    if (identity.TLS){
      alert(`Randomness fetched from https://`+ identity.Address + '/api/public\n\n' + JSON.stringify(JSON.parse(jsonStr),null,2));
    } else {
      alert(`Randomness fetched from http://`+ identity.Address + '/api/public\n\n' + JSON.stringify(JSON.parse(jsonStr),null,2));
    }
  };
  var p2 = document.createElement("p");

  var textnode = document.createTextNode(randomness_2lines);
  if (verified) {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " verified");
  } else {
    var textnode2 = document.createTextNode(round + ' @ ' + timestamp + " unverified");
  }
  p.appendChild(textnode);
  p2.appendChild(textnode2);
  var item = latestDiv.childNodes[0];
  var item = latestDiv.childNodes[1];
  latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  latestDiv.replaceChild(p2, latestDiv.childNodes[1]);


  //append previous to history
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

function print_nodes(identity) {
  if (nodesDiv.childElementCount == 0) {
    fetchGroup(identity).then(group => {
      var i = 0;
      while (i < group.Nodes.length) {
        let p4 = document.createElement("p");
        p4.onmouseover = function() { p4.style.textDecoration = "underline"; };
        p4.onmouseout = function() {p4.style.textDecoration = "none";};
        let addr = group.Nodes[i].Address;
        let tls = group.Nodes[i].TLS;
        p4.onclick = function() {
          window.identity = {Address: addr, TLS: tls};
          display_randomness();
          window.clearInterval(id);
          window.setInterval(display_randomness, 60000);
        };
        var text = document.createTextNode(group.Nodes[i].Address);
        p4.appendChild(text);
        nodesDiv.appendChild(p4);
        i = i + 1;
      }
    }).catch(error => console.error('Could not fetch group:', error))
  }
}
