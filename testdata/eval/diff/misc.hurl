GET {{base}}/json
HTTP 200
[Asserts]
jsonpath "$.date" toDate "%Y-%m-%dT%H:%M:%S%.fZ" format "%a, %d %b %Y %H:%M:%S" == "Fri, 02 Jan 2026 03:04:05"
jsonpath "$.date" toDate "%+" format "%Y-%m-%dT%H:%M:%S%.fZ" == "x"
jsonpath "$.date" toDate "%+" == "x"
jsonpath "$.date" toDate "%+" toString == "2026-01-02T03:04:05.123000Z"
jsonpath "$.date" toDate "%Y-%m-%d" == 1
jsonpath "$.date" toDate "%Y-%m-%dT%H:%M:%S%.fZ" format "%A %B %j %U %e %I %p %Z %z %s %y %C %G %V %u %w" == "x"
jsonpath "$.date" toDate "%+" format "%-d %_m %0e %3f %.6f %:z %T %D %F %R %r %c %v %h %k %l %P" == "x"
jsonpath "$.date" toDate "%+" isDate
jsonpath "$.date" toDate "%+" daysBeforeNow > 0
jsonpath "$.str" == "{{onef}}"
jsonpath "$.str" != "{{newUuid}}"
jsonpath "$.books[?search(@.title, 'u')].title" == "x"
jsonpath "$.books[?match(@.title, '[A-Z].*k')].title" == "Ubik"
jsonpath "$.books[?match(@.title, '\\w+')].title" count == 3
jsonpath "$.obj.c" == "x"
jsonpath "$.books" nth 0 == "x"
jsonpath "$.list" == "x"
jsonpath "$.list" nth 5 jsonpath "$.k" == "v"
jsonpath "$.int" jsonpath "$" == 1
jsonpath "$.list" count > 5.5
jsonpath "$.float2" == 2.50
jsonpath "$.exp" toString == "100.0"
jsonpath "$.huge" toString == "x"
jsonpath "$.huge" toFloat == 1
jsonpath "$.huge" == 1
jsonpath "$.list" isEmpty
jsonpath "$.empty" count == 0
jsonpath "$.list[3]" == null
jsonpath "$.list[3]" exists
jsonpath "$.list[3]" isEmpty
jsonpath "$.missing" isEmpty
jsonpath "$.missing" contains "x"
jsonpath "$.missing" matches "a"
jsonpath "$.missing" matches /a/
jsonpath "$.missing" toString == "x"
jsonpath "$.missing" > 1
jsonpath "$.missing" == hex,01;
jsonpath "$.missing" isString
jsonpath "$.missing" not isString
jsonpath "$.books[{{one}}].title" == "Emma"
jsonpath "$.books[{{onef}}].title" == "Emma"
jsonpath "$.books[{{nope}}].title" == "Emma"
jsonpath "$.str" == {{onef}}
jsonpath "$.list" includes {{one}}
jsonpath "$.list" not includes {{one}}
jsonpath "$.list" not contains 1
jsonpath "$.bool" == false
jsonpath "$.bool" not == false
jsonpath "$.bool" isBoolean
jsonpath "$.date" > "2025"
jsonpath "$.date" < "2025"
jsonpath "$.str" < "d"
jsonpath "$.str" > "cafe"
jsonpath "$.int" == true
jsonpath "$.nulls" != null
jsonpath "$.nulls" exists
jsonpath "$.obj" == "Object()"
jsonpath "$.obj" count == 3
jsonpath "$.list" nth {{onef}} == 2
jsonpath "$.list" nth {{one}} == 2
jsonpath "$.csv" split "," nth {{nope}} == 2
jsonpath "$.csv" regex "(" == "x"
jsonpath "$.str" matches "("
body == "x"
