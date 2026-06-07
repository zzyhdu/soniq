#!/usr/bin/env python3
import base64
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

EXPECTED_API_KEY = "test-api-key"
EXPECTED_MODEL = "mimo-v2.5-asr"
EXPECTED_LANGUAGE = "en"


class Handler(BaseHTTPRequestHandler):
    server_version = "SoniqFakeASR/1.0"

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404, "unexpected path")
            return
        if self.headers.get("api-key") != EXPECTED_API_KEY:
            self.send_error(401, "missing api-key")
            return
        content_type = self.headers.get("Content-Type", "")
        if "application/json" not in content_type:
            self.send_error(415, "expected application/json")
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(length))
            assert body["model"] == EXPECTED_MODEL
            assert body["asr_options"]["language"] == EXPECTED_LANGUAGE
            messages = body["messages"]
            assert len(messages) == 1
            assert messages[0]["role"] == "user"
            content = messages[0]["content"]
            assert len(content) == 1
            assert content[0]["type"] == "input_audio"
            data_url = content[0]["input_audio"]["data"]
            prefix = "data:audio/wav;base64,"
            assert data_url.startswith(prefix)
            audio = base64.b64decode(data_url[len(prefix):], validate=True)
            assert len(audio) > 0
        except Exception as exc:
            self.send_error(400, f"invalid request: {exc}")
            return
        response = {
            "id": "fake-asr-smoke",
            "model": EXPECTED_MODEL,
            "choices": [
                {"message": {"role": "assistant", "content": "Fake OpenAI-compatible ASR transcript from normalized audio."}}
            ],
        }
        raw = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, format, *args):
        print("[fake-asr] " + format % args, file=sys.stderr, flush=True)


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    port = str(server.server_address[1])
    if len(sys.argv) > 1:
        with open(sys.argv[1], "w", encoding="utf-8") as port_file:
            port_file.write(port)
    else:
        print(port, flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
