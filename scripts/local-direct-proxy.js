const http = require("http");
const net = require("net");
const { URL } = require("url");

const server = http.createServer((req, res) => {
  const u = new URL(req.url);
  const opts = { hostname: u.hostname, port: u.port || 80, path: u.pathname + u.search, method: req.method, headers: { ...req.headers, host: u.host } };
  delete opts.headers["proxy-connection"];
  const proxy = http.request(opts, (pres) => {
    res.writeHead(pres.statusCode, pres.headers);
    pres.pipe(res);
  });
  proxy.on("error", (e) => { res.writeHead(502); res.end(e.message); });
  req.pipe(proxy);
});

server.on("connect", (req, clientSocket, head) => {
  const [host, portStr] = req.url.split(":");
  const port = parseInt(portStr || "443", 10);
  const serverSocket = net.connect(port, host, () => {
    clientSocket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
    if (head && head.length) serverSocket.write(head);
    serverSocket.pipe(clientSocket);
    clientSocket.pipe(serverSocket);
  });
  serverSocket.on("error", () => clientSocket.end());
  clientSocket.on("error", () => serverSocket.end());
});

server.listen(10801, "127.0.0.1", () => console.log("direct proxy on 127.0.0.1:10801"));
