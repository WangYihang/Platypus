#!/usr/bin/env python3
"""Static file server with SPA fallback: when a path doesn't map to a file,
serve index.html so the client-side router handles the URL on refresh.

Usage: spa-serve.py [port] [root-dir]
"""
import os
import sys
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer


class SPAHandler(SimpleHTTPRequestHandler):
    def do_GET(self):
        url_path = self.path.split("?", 1)[0].split("#", 1)[0]
        fs_path = self.translate_path(url_path)
        # Fall back only for "route-like" paths (no file extension); missing
        # assets like /static/foo.js should still 404 honestly.
        if not os.path.exists(fs_path) and "." not in os.path.basename(url_path):
            self.path = "/index.html"
        return super().do_GET()


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 7777
    root = sys.argv[2] if len(sys.argv) > 2 else "."
    os.chdir(root)
    with ThreadingHTTPServer(("", port), SPAHandler) as httpd:
        print(f"SPA preview: http://localhost:{port}  (root: {os.path.abspath(root)})")
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
