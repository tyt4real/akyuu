package search

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DSLAST represents the parsed abstract syntax tree of a DSL query.
type DSLAST struct {
	Filters []Filter
	Sort    *Sort
	Limit   int
	Offset  int
}

// Filter represents a single filter condition.
type Filter struct {
	Field    string
	Operator string // ":", ":", ">", ">=", "<", "<=", "~" (contains), "!" (not)
	Value    any
	Negate   bool
}

// Sort represents a sort specification.
type Sort struct {
	Field string
	Order string // "asc" or "desc"
}

// Parser parses DSL query strings into an AST.
type Parser struct {
	input string
	pos   int
	len   int
}

// NewParser creates a new DSL parser.
func NewParser(input string) *Parser {
	return &Parser{input: strings.TrimSpace(input), len: len(input)}
}

// Parse parses the input string and returns an AST.
func (p *Parser) Parse() (*DSLAST, error) {
	ast := &DSLAST{
		Limit:  20,
		Offset: 0,
	}

	for p.pos < p.len {
		p.skipWhitespace()
		if p.pos >= p.len {
			break
		}

		// Check for special keywords: sort, limit, offset
		if p.matchKeyword("sort") || p.matchKeyword("order") {
			p.skipWhitespace()
			if p.pos < p.len && p.input[p.pos] == ':' {
				p.pos++
				p.skipWhitespace()
				field := p.parseIdentifier()
				p.skipWhitespace()
				order := "desc"
				if p.matchKeyword("asc") {
					order = "asc"
				} else if p.matchKeyword("desc") {
					order = "desc"
				}
				ast.Sort = &Sort{Field: field, Order: order}
				continue
			}
		}

		if p.matchKeyword("limit") {
			p.skipWhitespace()
			if p.pos < p.len && p.input[p.pos] == ':' {
				p.pos++
				p.skipWhitespace()
				limit, err := p.parseNumber()
				if err != nil {
					return nil, err
				}
				ast.Limit = limit
				continue
			}
		}

		if p.matchKeyword("offset") {
			p.skipWhitespace()
			if p.pos < p.len && p.input[p.pos] == ':' {
				p.pos++
				p.skipWhitespace()
				offset, err := p.parseNumber()
				if err != nil {
					return nil, err
				}
				ast.Offset = offset
				continue
			}
		}

		// Parse a filter
		filter, err := p.parseFilter()
		if err != nil {
			return nil, err
		}
		if filter != nil {
			ast.Filters = append(ast.Filters, *filter)
		}
	}

	return ast, nil
}

func (p *Parser) parseFilter() (*Filter, error) {
	// Check for negation
	negate := false
	if p.matchKeyword("not") || p.matchKeyword("!") {
		negate = true
		p.skipWhitespace()
	}

	// Check if the input starts with an operator (e.g., >=100, >5, ~text)
	// In this case, there's no explicit field - it's a value-only filter
	if p.pos < p.len {
		c := p.input[p.pos]
		if c == '>' || c == '<' || c == '~' || c == '!' || c == ':' || c == '=' {
			// No field specified, treat as free-text with default field
			return p.parseValueOnlyFilter(negate)
		}
	}

	// Parse field name
	field := p.parseIdentifier()
	if field == "" {
		// This might be a free-text search
		text := p.parseQuotedString()
		if text == "" {
			text = p.parseIdentifier()
		}
		if text != "" {
			return &Filter{
				Field:    "text",
				Operator: "~",
				Value:    text,
				Negate:   negate,
			}, nil
		}
		return nil, nil
	}

	// Check if there's an operator after the field
	// If not, this is free text (the field is actually the first word of the query)
	p.skipWhitespace()
	hasOperator := false
	if p.pos < p.len {
		c := p.input[p.pos]
		if c == ':' || c == '>' || c == '<' || c == '~' || c == '!' || c == '=' {
			hasOperator = true
		}
	}
	if !hasOperator {
		// No operator after field - this is free text starting with the field as first word
		// Consume the rest of the input as the free text value
		rest := strings.TrimSpace(p.input[p.pos:])
		if rest != "" {
			field = field + " " + rest
		}
		p.pos = p.len // consume all remaining input
		return &Filter{
			Field:    "text",
			Operator: "~",
			Value:    field,
			Negate:   negate,
		}, nil
	}
	p.skipWhitespace()
	if field == "has" && p.pos < p.len && p.input[p.pos] == ':' {
		p.pos++ // consume :
		p.skipWhitespace()
		subField := p.parseIdentifier()
		if subField != "" {
			field = "has_" + subField
		}
	}

	p.skipWhitespace()

	// Parse operator
	operator := ":"
	if p.pos < p.len {
		switch p.input[p.pos] {
		case ':', '>', '<', '~', '!':
			operator = string(p.input[p.pos])
			p.pos++
			if p.pos < p.len && p.input[p.pos] == '=' {
				operator += "="
				p.pos++
			}
		case '=':
			// Allow = as alias for :
			operator = ":"
			p.pos++
		}
	}

	p.skipWhitespace()

	// Parse value
	var value any
	var err error

	// Check for date/time
	if p.pos < p.len && (p.input[p.pos] == '"' || p.input[p.pos] == '\'') {
		value = p.parseQuotedString()
	} else if p.pos < p.len && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9') {
		// Could be number or date
		start := p.pos
		for p.pos < p.len && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || p.input[p.pos] == '-' || p.input[p.pos] == ':' || p.input[p.pos] == 'T' || p.input[p.pos] == 'Z' || p.input[p.pos] == '.') {
			p.pos++
		}
		token := p.input[start:p.pos]
		// Try parsing as time first
		if strings.Contains(token, "-") || strings.Contains(token, ":") {
			value, err = p.parseTime(token)
			if err != nil {
				// Fall back to number
				value, err = strconv.ParseFloat(token, 64)
			}
		} else {
			value, err = strconv.ParseFloat(token, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("parse value: %w", err)
		}
	} else {
		// Unquoted string/identifier
		value = p.parseIdentifier()
	}

	return &Filter{
		Field:    field,
		Operator: operator,
		Value:    value,
		Negate:   negate,
	}, nil
}

// parseValueOnlyFilter handles filters that start with an operator (e.g., >=100, ~text)
func (p *Parser) parseValueOnlyFilter(negate bool) (*Filter, error) {
	// Parse operator
	operator := "~" // default to contains
	if p.pos < p.len {
		switch p.input[p.pos] {
		case '>', '<', '~', '!':
			operator = string(p.input[p.pos])
			p.pos++
			if p.pos < p.len && p.input[p.pos] == '=' {
				operator += "="
				p.pos++
			}
		case '=':
			operator = ":"
			p.pos++
		case ':':
			operator = ":"
			p.pos++
		}
	}

	p.skipWhitespace()

	// Parse value
	var value any
	var err error

	if p.pos < p.len && (p.input[p.pos] == '"' || p.input[p.pos] == '\'') {
		value = p.parseQuotedString()
	} else if p.pos < p.len && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9') {
		start := p.pos
		for p.pos < p.len && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || p.input[p.pos] == '-' || p.input[p.pos] == ':' || p.input[p.pos] == 'T' || p.input[p.pos] == 'Z' || p.input[p.pos] == '.') {
			p.pos++
		}
		token := p.input[start:p.pos]
		if strings.Contains(token, "-") || strings.Contains(token, ":") {
			value, err = p.parseTime(token)
			if err != nil {
				value, err = strconv.ParseFloat(token, 64)
			}
		} else {
			value, err = strconv.ParseFloat(token, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("parse value: %w", err)
		}
	} else {
		value = p.parseIdentifier()
	}

	// For value-only filters, use "text" as default field
	return &Filter{
		Field:    "text",
		Operator: operator,
		Value:    value,
		Negate:   negate,
	}, nil
}

func (p *Parser) parseIdentifier() string {
	start := p.pos
	for p.pos < p.len {
		c := p.input[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' || c == '/' {
			p.pos++
		} else {
			break
		}
	}
	return strings.TrimSpace(p.input[start:p.pos])
}

func (p *Parser) parseQuotedString() string {
	if p.pos >= p.len {
		return ""
	}
	quote := p.input[p.pos]
	if quote != '"' && quote != '\'' {
		return ""
	}
	p.pos++ // skip opening quote
	start := p.pos
	for p.pos < p.len && p.input[p.pos] != quote {
		p.pos++
	}
	result := p.input[start:p.pos]
	if p.pos < p.len {
		p.pos++ // skip closing quote
	}
	return result
}

func (p *Parser) skipWhitespace() {
	for p.pos < p.len && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
}

func (p *Parser) parseNumber() (int, error) {
	start := p.pos
	for p.pos < p.len && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos {
		return 0, errors.New("expected number")
	}
	return strconv.Atoi(p.input[start:p.pos])
}

func (p *Parser) parseTime(token string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006/01/02",
		"01/02/2006",
		"2006-01",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, token); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid time format")
}

func (p *Parser) matchKeyword(keyword string) bool {
	if p.pos+len(keyword) <= p.len && strings.EqualFold(p.input[p.pos:p.pos+len(keyword)], keyword) {
		// Check word boundary - next char must not be identifier char
		end := p.pos + len(keyword)
		if end == p.len || !isIdentifierChar(p.input[end]) {
			p.pos = end
			return true
		}
	}
	return false
}

func isIdentifierChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// ToSQLWhere converts the AST to a SQL WHERE clause and arguments.
// Returns (whereClause, args, vectorParams).
func (ast *DSLAST) ToSQLWhere() (string, []any, map[string]any) {
	var conditions []string
	var args []any
	vectorParams := make(map[string]any)
	argIdx := 1

	for _, f := range ast.Filters {
		cond, vals, vp := f.toSQL(argIdx)
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, vals...)
			argIdx += len(vals)
			for k, v := range vp {
				vectorParams[k] = v
			}
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	return where, args, vectorParams
}

func (f *Filter) toSQL(argIdx int) (string, []any, map[string]any) {
	var vals []any
	vp := make(map[string]any)

	// Map DSL fields to SQL columns
	col := f.fieldToColumn()
	if col == "" {
		return "", nil, nil
	}

	// Check if the column is a raw SQL expression (like EXISTS subquery)
	// These don't take a value parameter
	isRawSQL := strings.HasPrefix(strings.ToUpper(strings.TrimSpace(col)), "EXISTS") ||
		strings.HasPrefix(strings.ToUpper(strings.TrimSpace(col)), "SELECT")

	// Check if this is a text field that should use FTS
	isTextField := f.Field == "text" || f.Field == "content" || f.Field == "body" || f.Field == "comment"

	// Handle special operators
	switch f.Operator {
	case "~": // Contains / full-text search
		if isTextField {
			// Full-text search on comment_tsv
			vals = append(vals, f.Value)
			cond := fmt.Sprintf("p.comment_tsv @@ plainto_tsquery($%d)", argIdx)
			if f.Negate {
				cond = "NOT (" + cond + ")"
			}
			return cond, vals, vp
		}
		// Fallback to ILIKE
		vals = append(vals, "%"+fmt.Sprint(f.Value)+"%")
		cond := fmt.Sprintf("%s ILIKE $%d", col, argIdx)
		if f.Negate {
			cond = "NOT (" + cond + ")"
		}
		return cond, vals, vp

	case ":", "=":
		if isTextField {
			// Text fields use FTS even with : operator
			vals = append(vals, f.Value)
			cond := fmt.Sprintf("p.comment_tsv @@ plainto_tsquery($%d)", argIdx)
			if f.Negate {
				cond = "NOT (" + cond + ")"
			}
			return cond, vals, vp
		}
		if isRawSQL {
			// For raw SQL (like has:image), the value is ignored
			// The expression itself is the condition
			if f.Negate {
				cond := fmt.Sprintf("NOT (%s)", col)
				return cond, vals, vp
			}
			return col, vals, vp
		}
		vals = append(vals, f.Value)
		cond := fmt.Sprintf("%s = $%d", col, argIdx)
		if f.Negate {
			cond = fmt.Sprintf("%s <> $%d", col, argIdx)
		}
		return cond, vals, vp

	case ">", ">=", "<", "<=":
		if isRawSQL {
			return "", nil, nil // Can't compare raw SQL
		}
		vals = append(vals, f.Value)
		cond := fmt.Sprintf("%s %s $%d", col, f.Operator, argIdx)
		if f.Negate {
			cond = "NOT (" + cond + ")"
		}
		return cond, vals, vp

	case "!":
		if isRawSQL {
			if f.Negate {
				return col, vals, vp
			}
			cond := fmt.Sprintf("NOT (%s)", col)
			return cond, vals, vp
		}
		vals = append(vals, f.Value)
		cond := fmt.Sprintf("%s <> $%d", col, argIdx)
		if f.Negate {
			cond = fmt.Sprintf("%s = $%d", col, argIdx)
		}
		return cond, vals, vp
	}

	return "", nil, nil
}

func (f *Filter) fieldToColumn() string {
	// Map DSL field names to SQL columns
	switch strings.ToLower(f.Field) {
	case "board":
		return "b.code"
	case "site":
		return "st.name"
	case "thread", "thread_id":
		return "t.thread_native_id"
	case "post", "post_id":
		return "p.post_native_id"
	case "author", "name", "tripcode":
		return "p.author_name"
	case "trip":
		return "p.tripcode"
	case "poster", "poster_id":
		return "p.poster_id"
	case "country":
		return "p.country"
	case "flag":
		return "p.flag"
	case "subject":
		return "t.subject"
	case "has_image", "hasimage", "image":
		return "EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id)"
	case "has_video", "hasvideo", "video":
		return "EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id AND f.mime_type LIKE 'video/%')"
	case "has_file", "hasfile", "file":
		return "EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id)"
	case "filename", "file_name":
		return "f.original_filename"
	case "before", "after", "date", "timestamp", "time":
		return "p.timestamp"
	case "sticky":
		return "t.sticky"
	case "locked":
		return "t.locked"
	case "archived":
		return "t.archived"
	case "sage":
		return "p.sage"
	case "text", "content", "body", "comment":
		return "p.comment_parsed"
	case "quote":
		return "EXISTS (SELECT 1 FROM post_quotes pq WHERE pq.post_id = p.id AND pq.quoted_post_native_id = $1)"
	case "vector", "embedding", "semantic":
		// This is handled at a higher level
		return "semantic"
	default:
		return ""
	}
}

// CompileDSL parses a DSL string and returns the SQL WHERE clause, args, and vector params.
func CompileDSL(dsl string) (string, []any, map[string]any, error) {
	p := NewParser(dsl)
	ast, err := p.Parse()
	if err != nil {
		return "", nil, nil, err
	}
	where, args, vp := ast.ToSQLWhere()
	return where, args, vp, nil
}
