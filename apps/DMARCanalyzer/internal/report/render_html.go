package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"time"
)

//go:embed templates/report.html.tmpl
var htmlTemplate string

// bar is one row of the inline SVG period chart.
type bar struct {
	Label        string
	Y            int
	PassW, FailW float64
	Pass, Fail   int64
	PassPct      float64
	TextX        float64
}

type htmlData struct {
	S      *Summary
	Range  string
	Tables []table
	Bars   []bar
	ChartH int
	ChartW int
	LabelW int
}

func renderHTML(w io.Writer, s *Summary) error {
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"f2":   func(f float64) string { return fmt.Sprintf("%.2f", f) },
		"date": func(t time.Time) string { return t.Format(time.RFC3339) },
	}).Parse(htmlTemplate)
	if err != nil {
		return err
	}
	const rowH, labelW, plotW = 26, 90, 560
	d := htmlData{S: s, Range: rangeText(s), Tables: tables(s), LabelW: labelW, ChartW: labelW + plotW + 120}
	var max int64
	for _, p := range s.Periods {
		if p.Messages > max {
			max = p.Messages
		}
	}
	for i, p := range s.Periods {
		b := bar{Label: p.Label, Y: i * rowH, Pass: p.Pass, Fail: p.Fail, PassPct: p.PassPct}
		if max > 0 {
			b.PassW = float64(p.Pass) / float64(max) * plotW
			b.FailW = float64(p.Fail) / float64(max) * plotW
		}
		b.TextX = float64(labelW) + b.PassW + b.FailW + 6
		d.Bars = append(d.Bars, b)
	}
	d.ChartH = len(d.Bars)*rowH + 4
	return tmpl.Execute(w, d)
}
