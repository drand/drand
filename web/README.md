# Drand Web App

## Requirements

- [hugo](https://gohugo.io)

## Commands

running 

```
python3 server_example/script.py
```
and opening 
```
http://localhost:8000/layouts/index.html
```
shows the web_page


``
hugo server -- start webserver on localhost:1313
``
cannot load the js files because of  `[Error] Refused to execute http://localhost:1313/static/js/util.js as script because "X-Content-Type: nosniff" was given and its Content-Type is not a script MIME type.`
