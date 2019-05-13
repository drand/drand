#!/usr/bin/env python3
from http.server import HTTPServer, SimpleHTTPRequestHandler, test
import webbrowser, os, sys

class CORSRequestHandler (SimpleHTTPRequestHandler):
    def end_headers (self):
        self.send_header('Access-Control-Allow-Origin', '*')
        SimpleHTTPRequestHandler.end_headers(self)

if __name__ == '__main__':
    #webbrowser.open('file://' + os.path.realpath("../index.html"))
    test(CORSRequestHandler, HTTPServer, port=int(sys.argv[1]) if len(sys.argv) > 1 else 8000)
