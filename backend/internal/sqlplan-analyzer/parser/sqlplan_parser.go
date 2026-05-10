// File: internal/parser/parser.go
// Purpose: SQL Server execution plan XML parser with streaming support
// Package: github.com/rsharma155/sqlplan-analyzer/internal/parser
package parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type Config struct {
	EnableStreaming bool
}

type Parser struct {
	config         Config
	operatorStack []*models.Operator
	planAnalysis  *models.PlanAnalysis
	warnings     []models.Warning
	missingIdx   []models.MissingIndex
	depth        int
	opIDCounter  int
	rootOp              *models.Operator
	currentMissingIdx       *models.MissingIndex
	currentColUsage       string
	missingIdxImpact      float64
	predicateType         string
	insideHashBuild       bool
	insideHashProbe       bool
	insideProbeResidual   bool
	insideBuildResidual   bool
}

func New(cfg Config) *Parser {
	return &Parser{config: cfg}
}

func (p *Parser) ParseFile(filepath string) (*models.PlanAnalysis, error) {
	file, err := os.Open(filepath)
	if err != nil { return nil, err }
	defer file.Close()
	return p.Parse(file)
}

func (p *Parser) ParseBytes(data []byte) (*models.PlanAnalysis, error) {
	return p.Parse(bytes.NewReader(data))
}

func (p *Parser) Parse(r io.Reader) (*models.PlanAnalysis, error) {
	p.planAnalysis = &models.PlanAnalysis{
		Metadata: models.QueryMetadata{},
		Operators: []models.Operator{},
		Warnings: []models.Warning{},
	}
	p.operatorStack = make([]*models.Operator, 0)
	p.warnings = make([]models.Warning, 0)
	p.missingIdx = make([]models.MissingIndex, 0)
	p.opIDCounter = 0
	p.rootOp = nil
	p.depth = 0

	data, err := io.ReadAll(r)
	if err != nil { return nil, err }
	data = convertToUTF8(data)

	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil 
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF { break }
		if err != nil { return nil, fmt.Errorf("failed to parse: %w", err) }

		switch t := token.(type) {
		case xml.StartElement:
			p.handleStartElement(t)
		case xml.EndElement:
			p.handleEndElement(t)
		}
	}

	if p.rootOp != nil {
		if p.planAnalysis.QueryPlan == nil { p.planAnalysis.QueryPlan = &models.QueryPlan{} }
		p.planAnalysis.QueryPlan.RelOp = p.rootOp
		p.planAnalysis.Operators = p.collectOperators(p.rootOp)
	}
	p.planAnalysis.Warnings = p.warnings
	p.planAnalysis.MissingIndexes = p.missingIdx
	return p.planAnalysis, nil
}

func (p *Parser) collectOperators(root *models.Operator) []models.Operator {
	if root == nil { return nil }
	res := []models.Operator{}
	q := []*models.Operator{root}
	for len(q) > 0 {
		curr := q[0]; q = q[1:]
		res = append(res, *curr)
		q = append(q, curr.Children...)
	}
	return res
}

func (p *Parser) handleStartElement(el xml.StartElement) {
	tag := strings.ToUpper(el.Name.Local)
	switch tag {
	case "QUERYPLAN": p.parseQueryPlan(el)
	case "RELOP": p.parseRelOp(el)
	case "INDEXSCAN", "INDEXSEEK": 
		if len(p.operatorStack) > 0 { p.operatorStack[len(p.operatorStack)-1].IndexScan = &models.IndexScan{} }
	case "TABLESCAN": 
		if len(p.operatorStack) > 0 { p.operatorStack[len(p.operatorStack)-1].TableScan = &models.TableScan{} }
	case "OBJECT": p.parseObject(el)
	case "STMTSIMPLE": p.parseStatement(el)
	case "RUNTIMECOUNTERSPERTHREAD": p.parseRuntimeCounters(el)
	case "WAIT": p.parseWait(el)
	case "HASHKEYSBUILD": p.insideHashBuild = true
	case "HASHKEYSPROBE": p.insideHashProbe = true
	case "PREDICATE": p.predicateType = "residual"
	case "PROBERESIDUAL": p.insideProbeResidual = true
	case "BUILDRESIDUAL": p.insideBuildResidual = true
	case "SCALAROPERATOR": p.parseScalarOperator(el)
	case "WARNINGS": p.parseWarnings(el)
	case "MISSINGINDEXGROUP": p.parseMissingIndexGroup(el)
	case "MISSINGINDEX": p.parseMissingIndex(el)
	case "COLUMNGROUP": p.parseColumnGroup(el)
	case "COLUMN": p.parseMissingIndexColumn(el)
	case "PLANAFFECTINGCONVERT": p.parsePlanAffectingConvert(el)
	case "SEEKPREDICATE", "SEEKPREDICATENEW": p.predicateType = "seek"
	}
}

func (p *Parser) handleEndElement(el xml.EndElement) {
	tag := strings.ToUpper(el.Name.Local)
	switch tag {
	case "RELOP":
		if len(p.operatorStack) > 0 {
			popped := p.operatorStack[len(p.operatorStack)-1]
			p.operatorStack = p.operatorStack[:len(p.operatorStack)-1]
			if len(p.operatorStack) > 0 {
				p.operatorStack[len(p.operatorStack)-1].Children = append(p.operatorStack[len(p.operatorStack)-1].Children, popped)
			} else { p.rootOp = popped }
		}
	case "MISSINGINDEX":
		if p.currentMissingIdx != nil {
			p.currentMissingIdx.Score = int(p.missingIdxImpact)
			p.missingIdx = append(p.missingIdx, *p.currentMissingIdx)
			p.currentMissingIdx = nil
		}
	case "HASHKEYSBUILD": p.insideHashBuild = false
	case "HASHKEYSPROBE": p.insideHashProbe = false
	case "PROBERESIDUAL": p.insideProbeResidual = false
	case "BUILDRESIDUAL": p.insideBuildResidual = false
	case "PREDICATE": p.predicateType = ""
	case "SEEKPREDICATE", "SEEKPREDICATENEW": p.predicateType = ""
	}
}

func (p *Parser) parseQueryPlan(el xml.StartElement) {
	if p.planAnalysis.QueryPlan == nil { p.planAnalysis.QueryPlan = &models.QueryPlan{} }
	for _, a := range el.Attr {
		val := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "DEGREEOFPARALLELISM": p.planAnalysis.QueryPlan.DegreeOfParallelism = parseInt(val)
		case "MEMORYGRANT": p.planAnalysis.QueryPlan.MemoryGrant = parseInt(val)
		case "CACHEDPLANSIZE": p.planAnalysis.QueryPlan.CachedPlanSize = parseInt(val)
		case "COMPILETIME": p.planAnalysis.QueryPlan.CompileTimeMs = parseInt(val)
		case "OPTIMIZATIONLEVEL": p.planAnalysis.QueryPlan.OptimizationLevel = val
		}
	}
}

func (p *Parser) parseRelOp(el xml.StartElement) {
	p.opIDCounter++
	op := &models.Operator{ID: p.opIDCounter, Children: make([]*models.Operator, 0)}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "PHYSICALOP": op.PhysicalOp = v
		case "LOGICALOP": op.LogicalOp = v
		case "ESTIMATEDTOTALSUBTREECOST": op.EstimatedTotalSubtreeCost = parseFloat(v)
		case "ESTIMATEROWS": op.EstimateRows = parseInt64(v)
		case "ACTUALROWS": op.ActualRows = parseInt64(v)
		}
	}
	p.operatorStack = append(p.operatorStack, op)
}

func (p *Parser) parseObject(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	op := p.operatorStack[len(p.operatorStack)-1]
	obj := models.IndexObject{}
	for _, a := range el.Attr {
		v := strings.Trim(a.Value, "[]")
		switch strings.ToUpper(a.Name.Local) {
		case "DATABASE": 
			obj.Database = v
			if p.planAnalysis.Metadata.DatabaseName == "" { p.planAnalysis.Metadata.DatabaseName = v }
		case "SCHEMA": obj.Schema = v
		case "TABLE": obj.Table = v
		case "INDEX": obj.Index = v
		}
	}
	if op.IndexScan != nil { op.IndexScan.Object = obj }
	if op.TableScan != nil { op.TableScan.Object = models.TableObject{Database: obj.Database, Schema: obj.Schema, Table: obj.Table} }
}

func (p *Parser) parseStatement(el xml.StartElement) {
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "STATEMENTTEXT": p.planAnalysis.Metadata.QueryText = v
		case "QUERYHASH": p.planAnalysis.Metadata.QueryHash = v
		}
	}
}

func (p *Parser) parseRuntimeCounters(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	rc := models.RuntimeCounter{}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "ACTUALROWS": rc.ActualRows = parseInt64(v)
		case "ACTUALEXECUTIONS": rc.ActualExecutions = parseInt(v)
		case "ACTUALCPUMS": rc.ActualCPUms = parseFloat(v)
		case "ACTUALELAPSEDMS": rc.ActualElapsedms = parseFloat(v)
		}
	}
	op := p.operatorStack[len(p.operatorStack)-1]
	op.RuntimeCounters = append(op.RuntimeCounters, rc)
}

func (p *Parser) parseWait(el xml.StartElement) {
	if p.planAnalysis.QueryPlan == nil { p.planAnalysis.QueryPlan = &models.QueryPlan{} }
	w := models.WaitStat{}
	for _, a := range el.Attr {
		val := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "WAITTYPE": w.WaitType = val
		case "WAITTIMEMS": w.WaitTimeMs = parseInt(val)
		case "WAITCOUNT": w.WaitCount = parseInt(val)
		}
	}
	p.planAnalysis.QueryPlan.WaitStats = append(p.planAnalysis.QueryPlan.WaitStats, w)
}

func (p *Parser) parseScalarOperator(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	var s string
	for _, a := range el.Attr {
		if strings.ToUpper(a.Name.Local) == "SCALARSTRING" { s = a.Value; break }
	}
	if s == "" { return }
	op := p.operatorStack[len(p.operatorStack)-1]
	if p.insideProbeResidual || p.insideBuildResidual {
		if op.Hash == nil { op.Hash = &models.HashMatch{} }
		if p.insideProbeResidual { op.Hash.ProbeResidual = s } else { op.Hash.BuildResidual = s }
	} else if p.predicateType == "residual" {
		if op.Predicate == nil { op.Predicate = &models.Predicate{} }
		op.Predicate.ScalarString = s
	} else if p.predicateType == "seek" {
		sp := models.SeekPredicate{SeekType: "Seek", PrefixPredicate: []models.PrefixPredicate{{ScalarString: s}}}
		op.SeekPredicates = append(op.SeekPredicates, sp)
	}
}

func (p *Parser) parseWarnings(el xml.StartElement) {
	for _, a := range el.Attr {
		if val := strings.ToLower(a.Value); val == "true" || val == "1" {
			name := a.Name.Local
			p.warnings = append(p.warnings, models.Warning{
				Type:     models.WarningType(name),
				Message:  name,
				Severity: models.SeverityMedium,
			})
		}
	}
}

func (p *Parser) parsePlanAffectingConvert(el xml.StartElement) {
	issue := ""
	expr := ""
	for _, a := range el.Attr {
		switch strings.ToUpper(a.Name.Local) {
		case "CONVERTISSUE": issue = a.Value
		case "EXPRESSION": expr = a.Value
		}
	}
	p.warnings = append(p.warnings, models.Warning{
		Type:     models.WarningTypeTypeConversion,
		Message:  fmt.Sprintf("Plan-affecting convert: %s (%s)", issue, expr),
		Severity: models.SeverityMedium,
	})
}

func (p *Parser) parseMissingIndexGroup(el xml.StartElement) {
	for _, a := range el.Attr {
		if strings.ToUpper(a.Name.Local) == "IMPACT" {
			p.missingIdxImpact = parseFloat(a.Value)
		}
	}
}

func (p *Parser) parseMissingIndex(el xml.StartElement) {
	mi := models.MissingIndex{ID: len(p.missingIdx) + 1}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "DATABASE": mi.Database = v
		case "SCHEMA": mi.Schema = v
		case "TABLE": mi.Table = v
		}
	}
	p.currentMissingIdx = &mi
}

func (p *Parser) parseColumnGroup(el xml.StartElement) {
	for _, a := range el.Attr {
		if strings.ToUpper(a.Name.Local) == "USAGE" {
			p.currentColUsage = strings.ToUpper(a.Value)
		}
	}
}

func (p *Parser) parseMissingIndexColumn(el xml.StartElement) {
	if p.currentMissingIdx == nil { return }
	var name string
	for _, a := range el.Attr {
		if strings.ToUpper(a.Name.Local) == "NAME" { name = a.Value }
	}
	if name == "" { return }
	switch p.currentColUsage {
	case "EQUALITY": p.currentMissingIdx.Columns = append(p.currentMissingIdx.Columns, models.MissingIndexColumn{Column: name, Equality: true})
	case "INEQUALITY": p.currentMissingIdx.Columns = append(p.currentMissingIdx.Columns, models.MissingIndexColumn{Column: name, Inequality: true})
	case "INCLUDE": p.currentMissingIdx.IncludedColumns = append(p.currentMissingIdx.IncludedColumns, name)
	}
}

func parseFloat(v string) float64 { f, _ := strconv.ParseFloat(v, 64); return f }
func parseInt64(v string) int64 { i, _ := strconv.ParseInt(v, 10, 64); return i }
func parseInt(v string) int { i, _ := strconv.Atoi(v); return i }

func convertToUTF8(data []byte) []byte {
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			res, _ := convertUTF16LEToUTF8(data[2:]); return res
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			res, _ := convertUTF16BEToUTF8(data[2:]); return res
		}
	}
	if bytes.IndexByte(data, 0) != -1 {
		res, _ := convertUTF16LEToUTF8(data); return res
	}
	return data
}

func convertUTF16LEToUTF8(data []byte) ([]byte, error) {
	res := make([]byte, 0, len(data))
	for i := 0; i < len(data)-1; i += 2 {
		c := uint16(data[i]) | (uint16(data[i+1]) << 8)
		if c < 0x80 { res = append(res, byte(c))
		} else if c < 0x800 { res = append(res, 0xC0|byte(c>>6), 0x80|byte(c&0x3F))
		} else { res = append(res, 0xE0|byte(c>>12), 0x80|byte((c>>6)&0x3F), 0x80|byte(c&0x3F)) }
	}
	return res, nil
}

func convertUTF16BEToUTF8(data []byte) ([]byte, error) {
	res := make([]byte, 0, len(data))
	for i := 0; i < len(data)-1; i += 2 {
		c := uint16(data[i+1]) | (uint16(data[i]) << 8)
		if c < 0x80 { res = append(res, byte(c))
		} else if c < 0x800 { res = append(res, 0xC0|byte(c>>6), 0x80|byte(c&0x3F))
		} else { res = append(res, 0xE0|byte(c>>12), 0x80|byte((c>>6)&0x3F), 0x80|byte(c&0x3F)) }
	}
	return res, nil
}
