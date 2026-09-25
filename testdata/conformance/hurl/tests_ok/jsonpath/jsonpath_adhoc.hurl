# Check JSONPPath Functions

# Functions do not raise runtime errors
# For example, an invalid regex used in the match function will not raise an error but simply return no match

GET http://localhost:8000/jsonpath/adhoc
HTTP 200
[Asserts]

# length function
jsonpath "$[?@.first_name == 'Bob'].last_name" == "Smith"
