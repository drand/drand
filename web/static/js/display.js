const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
var lastRound = "0";

function display_randomness(identity) {
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
  p.onclick = function() { alert(`Randomness requested to the node running at `+ identity.Address +`\n\n{\n"round": ` + round + `,\n"previous":"` + previous + `",\n"randomness": {\n     "gid": 21,    "point":"`+ randomness+ `"\n       }\n}`);};
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
}
