/**
* used to communicate with dns-js.com API
**/


DNS = {
  QueryType : {
    A : 1,
    NS : 2,
    HINFO : 13,
    AFSDB : 18,
    SRV : 33,
  },

  Query: function (domain, type, callback) {
    DNS._callApi({
      Action: "Query",
      Domain: domain,
      Type: type
    },
    callback);
  },
  _callApi: function (request, callback) {
    var xhr = new XMLHttpRequest();
    URL = "https://www.dns-js.com/api.aspx";
    xhr.open("POST", URL);
    xhr.onreadystatechange = function () {
      if (this.readyState === XMLHttpRequest.DONE && this.status === 200) {
        callback(JSON.parse(xhr.response));
      }
    }
    xhr.send(JSON.stringify(request));
  },

  Country: function (domain, type, callback) {
    DNS._callLoc({
      Action: "Query",
      Domain: domain,
      Type: type
    },
    callback);
  },
  _callLoc: async function (request, callback) {
    const response = await fetch('https://extreme-ip-lookup.com/json/128.179.133.69');
    console.log(response);
    const myJson = await response.json();
    console.log(JSON.stringify(myJson));
  }
};
