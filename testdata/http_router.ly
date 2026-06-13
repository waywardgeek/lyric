lyric http_example {
  import errors from "errors"
  import fmt from "fmt"

  // Simple HTTP-like request/response types
  enum Method { GET POST PUT DELETE }

  class Request {
    method: Method
    path: string
    body: string
  }
  class Response {
    status: i32
    body: string
  }

  // Example handlers
  func hello_handler(req: Request) -> (Response, error) {
    return (Response { status: 200, body: f"Hello from {req.path}!" }, nil)
  }

  func echo_handler(req: Request) -> (Response, error) {
    return (Response { status: 200, body: req.body }, nil)
  }

  func fail_handler(req: Request) -> (Response, error) {
    return (Response { status: 0, body: "" }, errors.New("handler failed"))
  }

  // Demonstrate ? operator with chaining
  func process(req: Request) -> (Response, error) {
    let resp = hello_handler(req)?
    return (resp, nil)
  }

  func main() {
    println("=== Lyric HTTP Example ===")

    let req1 = Request { method: GET, path: "/hello", body: "" }
    let (resp1, err1) = hello_handler(req1)
    if err1 == nil {
      println(f"Status: {resp1.status}, Body: {resp1.body}")
    }

    let req2 = Request { method: POST, path: "/echo", body: "Hello World" }
    let (resp2, err2) = echo_handler(req2)
    if err2 == nil {
      println(f"Status: {resp2.status}, Body: {resp2.body}")
    }

    let (resp4, err4) = process(req1)
    if err4 == nil {
      println(f"Process: {resp4.body}")
    }

    let (_, err5) = fail_handler(req1)
    if err5 != nil {
      println(f"Expected error: {err5}")
    }

    // Pattern match on enum
    let m = POST
    match m {
      GET => { println("GET request") }
      POST => { println("POST request") }
      _ => { println("Other method") }
    }
  }
}
