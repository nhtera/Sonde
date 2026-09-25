GET {{base}}/json
HTTP 200
[Asserts]
jsonpath "$.int" == 1
jsonpath "$.int" == 1.0
jsonpath "$.float" == 1
jsonpath "$.float" == 1.0
jsonpath "$.float" isFloat
jsonpath "$.float" isInteger
jsonpath "$.int" isInteger
jsonpath "$.exp" == 100
jsonpath "$.exp" isFloat
jsonpath "$.big" == 123456789012345678901234567890
jsonpath "$.big" isInteger
jsonpath "$.big" isNumber
jsonpath "$.big" > 9223372036854775807
jsonpath "$.big" > 1000
jsonpath "$.big2" < 123456789012345678901234567890
jsonpath "$.huge" isInteger
jsonpath "$.neg" < 0
jsonpath "$.neg" == -1.5
jsonpath "$.neg0" == 0
jsonpath "$.tiny" < 0.001
jsonpath "$.float2" > 2
jsonpath "$.float2" >= 2.5
jsonpath "$.float2" <= 2.4
jsonpath "$.int" != 2
jsonpath "$.int" == "1"
jsonpath "$.int" > "0"
jsonpath "$.float" == 2
jsonpath "$.big" == 1
jsonpath "$.tenth" toString == "0.1"
jsonpath "$.float" toString == "1.0"
jsonpath "$.neg" toString == "-1.5"
jsonpath "$.tiny" toString == "0.0000001"
jsonpath "$.big" toString == "123456789012345678901234567890"
jsonpath "$.int" toFloat == 1.0
jsonpath "$.float2" toInt == 2
jsonpath "$.neg" toInt == -1
jsonpath "$.big" toFloat == 1
jsonpath "$.big" toInt == 1
jsonpath "$.str" toInt == 1
jsonpath "$.list" count == 6
jsonpath "$.list[1]" == 2
jsonpath "$.list[1]" isFloat
jsonpath "$.missing" == 1
jsonpath "$.missing" not exists
jsonpath "$.float" == {{one}}
jsonpath "$.float" == {{onef}}
