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

  identity = {Address: "127.0.0.1:8000", TLS: false, Key: "7cadcddbc8b87c045decf45ba3b0cc4292a3bd496904cf8b61a1ea6f2eae82166f508b79479ee2df378aa5ac3f5b061d92335679dfd21f6c1b06f7aacf434ccf5a46e87cc2006a76b7dd03a6d0c9a6efd7786dd5fbe77926fa681f1b5385f7453afd47e7d3685747fb75fdbc02990509049d28a079cab444b37e8fc0459be111"}

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
