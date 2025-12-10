package synthesis

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (a *SynthesisAnalyzer) outputSynthesisReport() error {
	// 创建HTML报告
	reportFile := filepath.Join(a.config.OutputDir, "synthesis_report.html")
	f, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// 生成HTML报告
	fmt.Fprintf(f, `<!DOCTYPE html>
<html>
<head>
    <title>合成成功率分析报告</title>
    <script src="https://cdn.plot.ly/plotly-latest.min.js"></script>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .chart { margin: 20px 0; border: 1px solid #ddd; padding: 15px; }
        table { border-collapse: collapse; width: 100%%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .success { color: green; }
        .warning { color: orange; }
        .error { color: red; }
    </style>
</head>
<body>
    <h1>DNA合成成功率分析报告</h1>
    <p>分析时间: %s</p>
    <p>总reads数: %d</p>
    
    <h2>各参考序列合成成功率</h2>
    <table>
        <tr>
            <th>参考序列ID</th>
            <th>长度</th>
            <th>覆盖深度</th>
            <th>平均成功率</th>
            <th>最弱点位置</th>
            <th>最弱点成功率</th>
        </tr>`, time.Now().Format("2006-01-02 15:04:05"), a.totalReads)

	// 计算统计
	a.resultMutex.RLock()
	for refIdx, ref := range a.references {
		stats := a.results[refIdx]
		avgSuccess, weakestPos, weakestRate := a.calcStatistics(stats)

		fmt.Fprintf(f, `
        <tr>
            <td>%s</td>
            <td>%d</td>
            <td>%.1f</td>
            <td class="%s">%.2f%%</td>
            <td>%d</td>
            <td class="%s">%.2f%%</td>
        </tr>`,
			ref.ID, ref.Length,
			float64(a.getTotalReads(stats))/float64(ref.Length),
			getSuccessClass(avgSuccess), avgSuccess*100,
			weakestPos,
			getSuccessClass(weakestRate), weakestRate*100,
		)
	}
	a.resultMutex.RUnlock()

	fmt.Fprintf(f, `</table>`)

	// 生成JavaScript数据用于图表
	fmt.Fprintf(f, `
    <div id="charts"></div>
    <script>
        var data = %s;
        
        // 为每个参考序列创建图表
        for (var i = 0; i < data.length; i++) {
            var div = document.createElement('div');
            div.className = 'chart';
            div.id = 'chart' + i;
            document.getElementById('charts').appendChild(div);
            
            var trace = {
                x: data[i].positions,
                y: data[i].successRates,
                type: 'scatter',
                mode: 'lines+markers',
                name: data[i].refId,
                line: {color: 'rgb(55, 128, 191)'}
            };
            
            var layout = {
                title: '合成成功率: ' + data[i].refId,
                xaxis: {title: '位置'},
                yaxis: {title: '成功率', range: [0.8, 1.0]},
                shapes: [{
                    type: 'line',
                    x0: 0,
                    x1: data[i].length,
                    y0: 0.95,
                    y1: 0.95,
                    line: {color: 'red', width: 1, dash: 'dash'}
                }]
            };
            
            Plotly.newPlot('chart' + i, [trace], layout);
        }
    </script>
</body>
</html>`, a.generateChartData())

	// 同时生成CSV报告
	csvFile := filepath.Join(a.config.OutputDir, "synthesis_stats.csv")
	a.outputCSVReport(csvFile)

	return nil
}
