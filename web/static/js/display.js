const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
const nodesDiv = document.querySelector('#nodes');
window.identity = "";

var lastRound = "0";

function display_randomness() {
  let identity = window.identity;
  move();
  var date = new Date();
  var timestamp = date.toISOString();

  if (identity.Key == "") {

    fetchAndVerify(identity)
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
  } else {
    //identity = {Address: "127.0.0.1:8000", TLS: false, Key: "7cadcddbc8b87c045decf45ba3b0cc4292a3bd496904cf8b61a1ea6f2eae82166f508b79479ee2df378aa5ac3f5b061d92335679dfd21f6c1b06f7aacf434ccf5a46e87cc2006a76b7dd03a6d0c9a6efd7786dd5fbe77926fa681f1b5385f7453afd47e7d3685747fb75fdbc02990509049d28a079cab444b37e8fc0459be111"}

    fetchAndVerifyWithKey(identity)
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
  }
  print_nodes(identity);
}

function move() {
  var elem = document.getElementById("myBar");
  var width = 0;
  var id = setInterval(frame, 1);
  function frame() {
    if (width >= 100) {
      clearInterval(id);
    } else {
      width = width + 0.015;
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
  p.onclick = function() { alert(`Randomness requested to the node running at `+ identity.Address + '\n\n' + JSON.stringify(JSON.parse(jsonStr),null,2));};
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
          window.identity = {Address: addr, TLS: tls, Key: ""};
        };
        var text = document.createTextNode(group.Nodes[i].Address);
        p4.appendChild(text);
        nodesDiv.appendChild(p4);
        i = i + 1;
      }
    }).catch(error => console.error('Could not fetch group:', error))
  }
}
