GET {{base}}/headers
HTTP 200
[Asserts]
header "X-Multi" count == 2
header "X-Multi" == "one"
header "X-Multi" contains "two"
header "x-num" == "42"
header "X-Num" toInt == 42
header "X-Missing" exists
header "X-Missing" count == 1
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" format "%Y-%m-%d" == "2021-01-13"
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" == "2021-01-13"
header "X-Date" toDate "%Y" == 1
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" daysBeforeNow > 1000
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" daysAfterNow < 0
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" isDate
header "X-Date" toDate "%a, %d %b %Y %H:%M:%S GMT" format "%👻" == "x"
header "X-Date" format "%Y" == "x"
status == 200
status < 300
version startsWith "1"
url endsWith "/headers"
ip == "127.0.0.1"
duration < 60000
redirects count == 0
certificate "Subject" exists
variable "one" == 1
variable "nope" exists
GET {{base}}/cookies
HTTP 200
[Asserts]
cookie "LSID" == "DQAAAKEaem_vYg"
cookie "LSID[Value]" == "DQAAAKEaem_vYg"
cookie "LSID[Expires]" toString == "2021-01-13T22:23:01.000000Z"
cookie "HSID[Expires]" isDate
cookie "LSID[Max-Age]" not exists
cookie "HSID[Max-Age]" == 60
cookie "SSID[Max-Age]" exists
cookie "HSID[Domain]" == ".localhost"
cookie "LSID[Path]" == "/accounts"
cookie "LSID[Secure]" exists
cookie "HSID[Secure]" exists
cookie "HSID[HttpOnly]" exists
cookie "HSID[SameSite]" == "Lax"
cookie "NOPE" exists
cookie "LSID[Secure]" == "x"
GET {{base}}/bytes
HTTP 200
[Asserts]
bytes == hex,000102cafeff;
bytes count == 6
bytes startsWith hex,0001;
bytes endsWith hex,feff;
bytes contains hex,cafe;
bytes contains hex,beef;
bytes isEmpty
bytes toHex == "000102cafeff"
bytes == "x"
rawbytes count == 6
sha256 toHex == "x"
md5 toHex == "x"
bytes isList
bytes isCollection
bytes utf8Decode == "x"
body == "x"
GET {{base}}/gzip
HTTP 200
[Asserts]
body == "compressed café"
bytes count == 16
rawbytes count > 16
sha256 == hex,00;
GET {{base}}/latin1
HTTP 200
[Asserts]
body == "café crème"
bytes charsetDecode "iso-8859-1" == "café crème"
bytes charsetDecode "utf-8" == "x"
bytes charsetDecode "nope" == "x"
bytes decode "latin1" == "café crème"
bytes utf8Decode == "x"
GET {{base}}/badutf8
HTTP 200
[Asserts]
body == "x"
bytes utf8Decode == "a\u{fffd}\u{fffd}b"
GET {{base}}/empty
HTTP 200
[Asserts]
body isEmpty
bytes isEmpty
body == ""
jsonpath "$" exists
