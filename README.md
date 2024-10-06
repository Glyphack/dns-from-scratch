My solution to the ["Build Your Own DNS server" Challenge](https://app.codecrafters.io/courses/dns-server/overview).

In this challenge, you'll build a DNS server that's capable of parsing and
creating DNS packets, responding to DNS queries, handling various record types
and doing recursive resolve. Along the way we'll learn about the DNS protocol,
DNS packet format, root servers, authoritative servers, forwarding servers,
various record types (A, AAAA, CNAME, etc) and more.

## Usage

```
go run app/main.go
```

In another shell fire dig command:

```
dig @127.0.0.1 -p 2053 +noedns codecrafters.io
```

Result:

```
; <<>> DiG 9.10.6 <<>> @127.0.0.1 -p 2053 +noedns codecrafters.io
; (1 server found)
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 56568
;; flags: qr rd ra ad; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0
;; WARNING: Message has 448 extra bytes at end

;; QUESTION SECTION:
;codecrafters.io.               IN      A

;; ANSWER SECTION:
codecrafters.io.        3600    IN      A       76.76.21.21

;; Query time: 48 msec
;; SERVER: 127.0.0.1#2053(127.0.0.1)
;; WHEN: Sun Oct 06 11:07:34 CEST 2024
;; MSG SIZE  rcvd: 512
```
