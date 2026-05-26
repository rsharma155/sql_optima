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
	insideParamList     bool
	insideOptimizerStats bool
	// multi-statement batch tracking
	stmtRoots    []*models.Operator
	stmtTexts    []string
	stmtCosts    []float64
	stmtIDs      []string
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
	p.insideParamList = false
	p.insideOptimizerStats = false
	p.stmtRoots = nil
	p.stmtTexts = nil
	p.stmtCosts = nil
	p.stmtIDs = nil

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

	// Populate multi-statement batch metadata
	if len(p.stmtRoots) > 1 {
		p.planAnalysis.IsBatch = true
		stmts := make([]models.Statement, len(p.stmtRoots))
		totalBatchCost := 0.0
		for _, r := range p.stmtRoots {
			if r != nil { totalBatchCost += r.EstimatedTotalSubtreeCost }
		}
		for i, r := range p.stmtRoots {
			s := models.Statement{
				RootOperator: r,
			}
			if i < len(p.stmtIDs)   { s.StatementID = p.stmtIDs[i] }
			if i < len(p.stmtTexts) { s.StatementText = p.stmtTexts[i] }
			if i < len(p.stmtCosts) { s.SubTreeCost = p.stmtCosts[i] }
			if totalBatchCost > 0 && s.SubTreeCost > 0 {
				s.CostPercent = s.SubTreeCost / totalBatchCost * 100
			}
			if r != nil { s.Operators = p.collectOperators(r) }
			stmts[i] = s
		}
		p.planAnalysis.Statements = stmts
	}

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
	case "INDEXSCAN", "INDEXSEEK": p.parseIndexScanAttrs(el)
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
	case "COLUMN":
		if p.insideOptimizerStats { p.parseStatisticsInfo(el) } else { p.parseMissingIndexColumn(el) }
	case "COLUMNREFERENCE": p.parseColumnReference(el)
	case "PLANAFFECTINGCONVERT": p.parsePlanAffectingConvert(el)
	case "SEEKPREDICATE", "SEEKPREDICATENEW": p.predicateType = "seek"
	case "PARAMETERLIST": p.insideParamList = true
	case "OPTIMIZERSTATSUSAGE": p.insideOptimizerStats = true
	case "STATISTICSINFO": p.parseStatisticsInfo(el)
	case "ADAPTIVEJOIN": p.parseAdaptiveJoin(el)
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
			} else {
				// Track each root per statement
				p.stmtRoots = append(p.stmtRoots, popped)
				p.rootOp = popped
			}
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
	case "PARAMETERLIST": p.insideParamList = false
	case "OPTIMIZERSTATSUSAGE": p.insideOptimizerStats = false
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
		case "NONPARALLELPLANREASON": p.planAnalysis.QueryPlan.NonParallelPlanReason = val
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
		case "ESTIMATEROWS": op.EstimateRows = parseFloat(v)
		case "ESTIMATEDROWSREAD": op.EstimatedRowsRead = parseFloat(v)
		case "AVGROWSIZE": op.AvgRowSize = parseFloat(v)
		case "ACTUALROWS": op.ActualRows = parseInt64(v)
		case "NODEID": op.NodeID = parseInt(v)
		case "ESTIMATEDEXECUTIONMODE": op.EstimatedExecutionMode = v
		case "STORAGE": op.Storage = v
		case "PARALLEL":
			op.Parallel = v == "1" || strings.ToLower(v) == "true"
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
	var stmtID, stmtText string
	var stmtCost float64
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "STATEMENTTEXT":
			stmtText = v
			if p.planAnalysis.Metadata.QueryText == "" { p.planAnalysis.Metadata.QueryText = v }
		case "QUERYHASH": p.planAnalysis.Metadata.QueryHash = v
		case "STATEMENTID": stmtID = v
		case "STATEMENTSUBTREECOST": stmtCost = parseFloat(v)
		}
	}
	p.stmtTexts = append(p.stmtTexts, stmtText)
	p.stmtIDs = append(p.stmtIDs, stmtID)
	p.stmtCosts = append(p.stmtCosts, stmtCost)
}

func (p *Parser) parseRuntimeCounters(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	rc := models.RuntimeCounter{}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "THREAD": rc.Thread = parseInt(v)
		case "ACTUALROWS": rc.ActualRows = parseInt64(v)
		case "ACTUALROWSREAD": rc.ActualRowsRead = parseInt64(v)
		case "ACTUALEXECUTIONS": rc.ActualExecutions = parseInt(v)
		case "ACTUALCPUMS": rc.ActualCPUms = parseFloat(v)
		case "ACTUALELAPSEDMS": rc.ActualElapsedms = parseFloat(v)
		case "ACTUALLOGICALREADS": rc.ActualLogicalReads = parseInt64(v)
		case "ACTUALPHYSICALREADS": rc.ActualPhysicalReads = parseInt64(v)
		case "ACTUALREBINDS": rc.ActualRebinds = parseInt64(v)
		case "ACTUALREWINDS": rc.ActualRewinds = parseInt64(v)
		case "ACTIVEPARALLELTHREAD": rc.ActiveParallelThread = parseInt(v)
		}
	}
	op := p.operatorStack[len(p.operatorStack)-1]
	op.RuntimeCounters = append(op.RuntimeCounters, rc)

	// Aggregate runtime counters into operator-level fields
	op.ActualRows += rc.ActualRows
	op.ActualExecutions = rc.ActualExecutions
	op.ActualCPUms += rc.ActualCPUms
	op.ActualElapsedms = rc.ActualElapsedms // last thread wins for elapsed
	op.ActualLogicalReads += rc.ActualLogicalReads
	op.ActualPhysicalReads += rc.ActualPhysicalReads
}

func (p *Parser) parseWait(el xml.StartElement) {
	w := models.WaitStat{}
	for _, a := range el.Attr {
		val := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "WAITTYPE": w.WaitType = val
		case "WAITTIMEMS": w.WaitTimeMs = parseInt(val)
		case "WAITCOUNT": w.WaitCount = parseInt(val)
		}
	}
	// Per-operator wait stats when inside a RelOp; otherwise plan-level
	if len(p.operatorStack) > 0 {
		op := p.operatorStack[len(p.operatorStack)-1]
		op.OpWaitStats = append(op.OpWaitStats, w)
	} else {
		if p.planAnalysis.QueryPlan == nil { p.planAnalysis.QueryPlan = &models.QueryPlan{} }
		p.planAnalysis.QueryPlan.WaitStats = append(p.planAnalysis.QueryPlan.WaitStats, w)
	}
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

// parseIndexScanAttrs initialises IndexScan on the top operator and captures
// Ordered, ForcedIndex, IndexKind from the element attributes.
func (p *Parser) parseIndexScanAttrs(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	op := p.operatorStack[len(p.operatorStack)-1]
	if op.IndexScan == nil { op.IndexScan = &models.IndexScan{} }
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "ORDERED":
			op.IndexScan.Ordered = v == "1" || strings.ToLower(v) == "true"
		case "FORCEDINDEX":
			op.IndexScan.ForcedIndex = v == "1" || strings.ToLower(v) == "true"
		case "INDEXKIND": op.IndexScan.IndexKind = v
		case "SCANTYPE":  op.IndexScan.ScanType = v
		case "STORAGE":   op.IndexScan.Storage = v
		}
	}
}

// parseColumnReference handles ColumnReference elements for hash-key lists and parameter lists.
func (p *Parser) parseColumnReference(el xml.StartElement) {
	if p.insideParamList {
		p.parseParamColumnRef(el)
		return
	}
	if len(p.operatorStack) == 0 { return }
	op := p.operatorStack[len(p.operatorStack)-1]
	if !p.insideHashBuild && !p.insideHashProbe { return }
	ref := models.ColumnReference{}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "DATABASE": ref.Database = v
		case "SCHEMA":   ref.Schema = v
		case "TABLE":    ref.Table = v
		case "ALIAS":    ref.Alias = v
		case "COLUMN":   ref.Column = v
		}
	}
	if op.Hash == nil { op.Hash = &models.HashMatch{} }
	if p.insideHashBuild {
		op.Hash.HashKeysBuild = append(op.Hash.HashKeysBuild, ref)
	} else {
		op.Hash.HashKeysProbe = append(op.Hash.HashKeysProbe, ref)
	}
}

// parseParamColumnRef reads compiled/runtime parameter values from ParameterList/ColumnReference.
func (p *Parser) parseParamColumnRef(el xml.StartElement) {
	var col, dataType, compiled, runtime string
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "COLUMN": col = v
		case "PARAMETERDATATYPE": dataType = v
		case "PARAMETERCOMPILEDVALUE": compiled = v
		case "PARAMETERRUNTIMEVALUE": runtime = v
		}
	}
	if compiled != "" {
		p.planAnalysis.CompiledParameters = append(p.planAnalysis.CompiledParameters,
			models.ParameterInfo{Parameter: col, DataType: dataType, Value: compiled})
	}
	if runtime != "" {
		p.planAnalysis.RuntimeParameters = append(p.planAnalysis.RuntimeParameters,
			models.ParameterInfo{Parameter: col, DataType: dataType, Value: runtime})
	}
}

// parseStatisticsInfo reads a StatisticsInfo element into plan-level or operator-level stats.
func (p *Parser) parseStatisticsInfo(el xml.StartElement) {
	info := models.OperatorStatsInfo{}
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "STATISTICS":        info.Object = v
		case "TABLE":
			if info.Object == "" { info.Object = v }
		case "LASTUPDATE":        info.LastUpdate = v
		case "MODIFICATIONCOUNT": info.ModificationCount = parseInt64(v)
		case "SAMPLINGPERCENT":   info.SamplingPercent = parseFloat(v)
		case "STATISTICSROWS":    info.TableCardinality = parseInt64(v)
		}
	}
	if info.Object == "" { return }
	if len(p.operatorStack) > 0 {
		op := p.operatorStack[len(p.operatorStack)-1]
		op.OpStatisticsInfo = append(op.OpStatisticsInfo, info)
	} else {
		p.planAnalysis.StatisticsInfo = append(p.planAnalysis.StatisticsInfo, info)
	}
}

// parseAdaptiveJoin reads AdaptiveJoin attributes.
func (p *Parser) parseAdaptiveJoin(el xml.StartElement) {
	if len(p.operatorStack) == 0 { return }
	op := p.operatorStack[len(p.operatorStack)-1]
	for _, a := range el.Attr {
		v := a.Value
		switch strings.ToUpper(a.Name.Local) {
		case "ADAPTIVETHRESHOLDROWS": op.AdaptiveThresholdRows = parseFloat(v)
		}
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
