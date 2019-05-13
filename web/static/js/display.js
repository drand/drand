const div = document.querySelector('#printDiv');
var lastRound = "0";

function testFetchKeyAndPublic() {
  move();
  var date = new Date();
  var timestamp = date.toISOString();
  identity = {Address: "127.0.0.1:8000", TLS: false}
  fetchAndVerify(identity)
    .then(function (fulfilled) {
      randomness = fulfilled[0]
      round = fulfilled[1]
      if (round == lastRound || round == undefined) {
        return
      }
      lastRound = round;
      console.log(round);
      console.log(randomness);

      var p = document.createElement("p");
      var p2 = document.createElement("p");
      var textnode = document.createTextNode(randomness);
      var textnode2 = document.createTextNode(round + ' @ ' + timestamp);
      //var textnode = document.createTextNode('(' + round + ') ' + randomness + ' : verified.');
      p.appendChild(textnode);
      p2.appendChild(textnode2);
      div.appendChild(p);
      div.appendChild(p2);
  })
    .catch(function (error) {
      randomness = error[0]
      round = error[1]
      if (round == lastRound || round == undefined) {
        return
      }
      lastRound = round;
      var textnode = document.createTextNode('(' + round + ') ' + randomness + ' : unverified.');
      var p = document.createElement("p");
      p.appendChild(textnode);
      div.appendChild(p);
  });
}

function testFetchAndVerifyWithKey() {
  move();
  identity = {Address: "127.0.0.1:8000", TLS: false, Key: "017f2254bc09a999661f92457122613adb773a6b7c74333a59bde7dd552a7eac2a79263bb6fb1f3840218f3181218b952e2af35be09edaee66566b458c92609f7571e8bb519c9109055b84f392c9e84f5bb828f988ce0423ce708be1dcf808d9cc63a610352b504115ee38bc23dd259e88a5d1221d53e45c9520be9b601fb4f578"}
  fetchAndVerifyWithKey(identity)
    .then(function (fulfilled) {
      randomness = fulfilled[0]
      round = fulfilled[1]
      if (round == lastRound || round == undefined) {
        return
      }
      lastRound = round;
      var textnode = document.createTextNode('(' + round + ') ' + randomness + ' : verified.');
      var p = document.createElement("p");
      p.appendChild(textnode);
      div.appendChild(p);
  })
    .catch(function (error) {
      randomness = error[0]
      round = error[1]
      if (round == lastRound || round == undefined) {
        return
      }
      lastRound = round;
      var textnode = document.createTextNode('(' + round + ') ' + randomness + ' : not verified.');
      var p = document.createElement("p");
      p.appendChild(textnode);
      div.appendChild(p);
  });
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
