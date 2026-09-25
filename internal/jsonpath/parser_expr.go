// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

// parseLogicalOrExpr parses a "||"-separated list of and-expressions.
func parseLogicalOrExpr(r *reader) (logicalExpr, error) {
	first, err := parseLogicalAndExpr(r)
	if err != nil {
		return logicalExpr{}, err
	}
	operands := []logicalExpr{first}
	for {
		skipWhitespace(r)
		if !matchStr("||", r) {
			break
		}
		skipWhitespace(r)
		next, err := parseLogicalAndExpr(r)
		if err != nil {
			return logicalExpr{}, err
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		return operands[0], nil
	}
	return logicalExpr{kind: exprOr, operands: operands}, nil
}

// parseLogicalAndExpr parses a "&&"-separated list of basic expressions.
func parseLogicalAndExpr(r *reader) (logicalExpr, error) {
	first, err := parseBasicExpr(r)
	if err != nil {
		return logicalExpr{}, err
	}
	operands := []logicalExpr{first}
	for {
		skipWhitespace(r)
		if !matchStr("&&", r) {
			break
		}
		skipWhitespace(r)
		next, err := parseBasicExpr(r)
		if err != nil {
			return logicalExpr{}, err
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		return operands[0], nil
	}
	return logicalExpr{kind: exprAnd, operands: operands}, nil
}

func parseBasicExpr(r *reader) (logicalExpr, error) {
	save := r.cursor()

	if expr, ok, err := tryParenExpr(r); err != nil {
		return logicalExpr{}, err
	} else if ok {
		return expr, nil
	}
	if cmp, ok, err := tryComparisonExpr(r); err != nil {
		return logicalExpr{}, err
	} else if ok {
		return logicalExpr{kind: exprComparison, cmp: &cmp}, nil
	}
	if expr, ok, err := tryTestExpr(r); err != nil {
		return logicalExpr{}, err
	} else if ok {
		return expr, nil
	}

	return logicalExpr{}, newParseError(save, "expecting basic expression")
}

func tryParenExpr(r *reader) (logicalExpr, bool, error) {
	save := r.cursor()
	not := matchStr("!", r)
	skipWhitespace(r)
	if !matchStr("(", r) {
		r.seek(save)
		return logicalExpr{}, false, nil
	}
	expr, err := parseLogicalOrExpr(r)
	if err != nil {
		return logicalExpr{}, false, err
	}
	if !matchStr(")", r) {
		return logicalExpr{}, false, newParseError(r.cursor(), "expecting ')'")
	}
	if not {
		return logicalExpr{kind: exprNot, notExpr: &expr}, true, nil
	}
	return expr, true, nil
}

func tryTestExpr(r *reader) (logicalExpr, bool, error) {
	not := matchStr("!", r)
	skipWhitespace(r)
	if query, ok, err := tryFilterQuery(r); err != nil {
		return logicalExpr{}, false, err
	} else if ok {
		return logicalExpr{kind: exprTest, testNot: not, testQuery: &query}, true, nil
	}
	if fn, ok, err := tryLogicalTypeFunction(r); err != nil {
		return logicalExpr{}, false, err
	} else if ok {
		return logicalExpr{kind: exprTest, testNot: not, testFn: &fn}, true, nil
	}
	return logicalExpr{}, false, nil
}
