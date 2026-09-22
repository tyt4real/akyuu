package search

import (
	"testing"
	"time"
)

func TestParseBasicFilter(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
		check   func(*testing.T, *DSLAST)
	}{
		{
			input:   "board:g",
			wantLen: 1,
			check: func(t *testing.T, ast *DSLAST) {
				if len(ast.Filters) != 1 {
					t.Fatalf("expected 1 filter, got %d", len(ast.Filters))
				}
				f := ast.Filters[0]
				if f.Field != "board" || f.Value != "g" || f.Operator != ":" {
					t.Errorf("filter = %+v, want board:=g", f)
				}
			},
		},
		{
			input:   "site:fourchan board:pol",
			wantLen: 2,
			check: func(t *testing.T, ast *DSLAST) {
				if len(ast.Filters) != 2 {
					t.Fatalf("expected 2 filters, got %d", len(ast.Filters))
				}
			},
		},
		{
			input:   "before:2024-01-01",
			wantLen: 1,
			check: func(t *testing.T, ast *DSLAST) {
				f := ast.Filters[0]
				if f.Field != "before" {
					t.Errorf("field = %q, want before", f.Field)
				}
				if _, ok := f.Value.(time.Time); !ok {
					t.Errorf("value type = %T, want time.Time", f.Value)
				}
			},
		},
		{
			input:   "has:image",
			wantLen: 1,
			check: func(t *testing.T, ast *DSLAST) {
				f := ast.Filters[0]
				if f.Field != "has_image" && f.Field != "hasimage" && f.Field != "image" {
					t.Errorf("field = %q", f.Field)
				}
			},
		},
		{
			input:   "filename:foo.jpg",
			wantLen: 1,
			check: func(t *testing.T, ast *DSLAST) {
				f := ast.Filters[0]
				if f.Value != "foo.jpg" {
					t.Errorf("value = %v, want foo.jpg", f.Value)
				}
			},
		},
		{
			input:   "text:hello world",
			wantLen: 1,
			check: func(t *testing.T, ast *DSLAST) {
				f := ast.Filters[0]
				if f.Field != "text" {
					t.Errorf("field = %q, want text", f.Field)
				}
			},
		},
		{
			input:   "limit:50",
			wantLen: 0,
			check: func(t *testing.T, ast *DSLAST) {
				if ast.Limit != 50 {
					t.Errorf("limit = %d, want 50", ast.Limit)
				}
			},
		},
		{
			input:   "sort:timestamp asc",
			wantLen: 0,
			check: func(t *testing.T, ast *DSLAST) {
				if ast.Sort == nil || ast.Sort.Field != "timestamp" || ast.Sort.Order != "asc" {
					t.Errorf("sort = %+v", ast.Sort)
				}
			},
		},
		{
			input:   "offset:100",
			wantLen: 0,
			check: func(t *testing.T, ast *DSLAST) {
				if ast.Offset != 100 {
					t.Errorf("offset = %d, want 100", ast.Offset)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			ast, err := p.Parse()
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, ast)
			}
		})
	}
}

func TestParseNegation(t *testing.T) {
	p := NewParser("not board:g")
	ast, err := p.Parse()
	if err != nil {
		t.Fatal(err)
	}
	if len(ast.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(ast.Filters))
	}
	if !ast.Filters[0].Negate {
		t.Error("expected negated filter")
	}
}

func TestParseFreeText(t *testing.T) {
	p := NewParser("hello world")
	ast, err := p.Parse()
	if err != nil {
		t.Fatal(err)
	}
	if len(ast.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(ast.Filters))
	}
	if ast.Filters[0].Field != "text" {
		t.Errorf("field = %q, want text", ast.Filters[0].Field)
	}
}

func TestParseQuotedString(t *testing.T) {
	p := NewParser(`board:"gaming" text:"hello world"`)
	ast, err := p.Parse()
	if err != nil {
		t.Fatal(err)
	}
	if len(ast.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(ast.Filters))
	}
	if ast.Filters[0].Value != "gaming" {
		t.Errorf("board value = %v, want gaming", ast.Filters[0].Value)
	}
	if ast.Filters[1].Value != "hello world" {
		t.Errorf("text value = %v, want hello world", ast.Filters[1].Value)
	}
}

func TestParseOperators(t *testing.T) {
	tests := []struct {
		input     string
		wantOp    string
		wantField string
	}{
		{">=100", ">=", ""},
		{"timestamp>1000", ">", "timestamp"},
		{"replies>=5", ">=", "replies"},
		{"board!=g", "!=", "board"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			ast, err := p.Parse()
			if err != nil {
				t.Fatal(err)
			}
			if len(ast.Filters) != 1 {
				t.Fatalf("expected 1 filter, got %d", len(ast.Filters))
			}
			f := ast.Filters[0]
			if f.Operator != tt.wantOp {
				t.Errorf("operator = %q, want %q", f.Operator, tt.wantOp)
			}
			if tt.wantField != "" && f.Field != tt.wantField {
				t.Errorf("field = %q, want %q", f.Field, tt.wantField)
			}
		})
	}
}

func TestToSQLWhere(t *testing.T) {
	tests := []struct {
		input       string
		wantWhere   string
		wantArgsLen int
	}{
		{
			input:       "board:g",
			wantWhere:   "WHERE b.code = $1",
			wantArgsLen: 1,
		},
		{
			input:       "site:fourchan board:pol",
			wantWhere:   "WHERE st.name = $1 AND b.code = $2",
			wantArgsLen: 2,
		},
		{
			input:       "has:image",
			wantWhere:   "WHERE EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id)",
			wantArgsLen: 0,
		},
		{
			input:       "text:hello",
			wantWhere:   "WHERE p.comment_tsv @@ plainto_tsquery($1)",
			wantArgsLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			ast, err := p.Parse()
			if err != nil {
				t.Fatal(err)
			}
			where, args, _ := ast.ToSQLWhere()
			if where != tt.wantWhere {
				t.Errorf("where = %q\nwant   %q", where, tt.wantWhere)
			}
			if len(args) != tt.wantArgsLen {
				t.Errorf("args len = %d, want %d", len(args), tt.wantArgsLen)
			}
		})
	}
}

func TestCompileDSL(t *testing.T) {
	where, args, vp, err := CompileDSL("board:g has:image before:2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if where == "" {
		t.Error("expected non-empty where clause")
	}
	if len(args) == 0 {
		t.Error("expected args for date filter")
	}
	_ = vp
}
