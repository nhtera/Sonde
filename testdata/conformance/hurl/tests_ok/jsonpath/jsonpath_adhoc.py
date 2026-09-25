from app import app
from flask import Response


@app.route("/jsonpath/adhoc")
def jsonpath_adhoc():
    return Response(
        """[
  {
    "first_name": "Bob",
    "last_name": "Smith"
  }
]""",
        mimetype="application/json",
    )
