package stats

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
)

// 更新输出函数以使用新的数据结构

// writePositionStats 写入各bam各位置各碱基变化组合的个数分布统计
func writePositionStats(stats *MutationStats, outputDir string) error {
	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_position_stats.csv", sampleName))
		filename2 := filepath.Join(outputDir, fmt.Sprintf("%s_position_stats2.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()
		file2, err := os.Create(filename2)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename2, err)
		}
		defer file2.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Sample,Position,Ref>Alt,Count,TotalReads,AlignedReads,ReadsWithMutations\n")
		writer2 := bufio.NewWriter(file2)
		writer2.WriteString("Sample,Position,Count,TotalReads,AlignedReads,ReadsWithMutations\n")

		// 收集所有位置
		var positions []string
		for pos := range sampleStats.PositionDetails {
			positions = append(positions, pos)
		}

		// 按位置排序
		sort.Slice(positions, func(i, j int) bool {
			numI, _ := strconv.Atoi(positions[i])
			numJ, _ := strconv.Atoi(positions[j])
			return numI < numJ
		})

		for _, pos := range positions {
			// 收集该位置的所有突变
			var mutations []string
			for mut := range sampleStats.PositionDetails[pos] {
				mutations = append(mutations, mut)
			}
			sort.Strings(mutations)

			count2 := 0
			for _, mut := range mutations {
				count := sampleStats.PositionDetails[pos][mut]
				fmt.Fprintf(writer, "%s,%s,%s,%d,%d,%d,%d\n",
					sampleName, pos, mut, count,
					sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.SubstitutionReads,
				)
				count2 += count
			}
			fmt.Fprintf(writer2, "%s,%s,%d,%d,%d,%d\n",
				sampleName, pos, count2,
				sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.SubstitutionReads,
			)
		}

		writer.Flush()
		writer2.Flush()
		sampleStats.RUnlock()
		slog.Debug("已写入", "filename", filename, "filename2", filename2)
	}

	return nil
}

// writeSampleMutationStats 写入各bam各碱基变化组合的分布
func writeSampleMutationStats(stats *MutationStats, outputDir string) error {
	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_mutation_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Sample,Ref>Alt,Count,TotalReads,,AlignedReads,ReadsWithMutations\n")

		// 收集所有突变
		var mutations []string
		for mut := range sampleStats.SubstitutionCount {
			mutations = append(mutations, mut)
		}
		sort.Strings(mutations)

		for _, mut := range mutations {
			count := sampleStats.SubstitutionCount[mut]
			fmt.Fprintf(writer, "%s,%s,%d,%d,%d,%d\n",
				sampleName, mut, count,
				sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.SubstitutionReads,
			)
		}

		writer.Flush()
		sampleStats.RUnlock()
		slog.Debug("已写入", "filename", filename)
	}

	return nil
}

// writeTotalMutationStats 写入总碱基变化组合的分布
func writeTotalMutationStats(stats *MutationStats, outputDir string) error {
	filename := filepath.Join(outputDir, "total_mutation_stats.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Ref>Alt,Count,TotalReads,TotalReadsWithMutations\n")

	// 收集所有突变
	var mutations []string
	for mut := range stats.SubstitutionCount {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	// 获取所有样本的总reads数和包含突变reads数的和
	totalReads := stats.TotalReadCount
	totalReadsWithMuts := stats.TotalReadsWithMuts

	for _, mut := range mutations {
		count := stats.SubstitutionCount[mut]
		fmt.Fprintf(writer, "%s,%d,%d,%d\n",
			mut, count, totalReads, totalReadsWithMuts)
	}

	writer.Flush()
	fmt.Printf("总统计已写入: %s\n", filename)

	return nil
}

// writeSummaryReport 写入汇总报告 - 更新版本
func writeSummaryReport(stats *MutationStats, outputDir string) error {
	// 1. 样本汇总（突变类型统计）
	filename := filepath.Join(outputDir, "summary_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 收集所有突变类型（包括通过MD发现的）
	allMutations := make(map[string]bool)
	for _, sampleStats := range stats.Samples {
		sampleStats.RLock()
		for mut := range sampleStats.SubstitutionCount {
			allMutations[mut] = true
		}
		sampleStats.RUnlock()
	}

	// 转换为排序的切片
	var mutationTypes []string
	for mut := range allMutations {
		mutationTypes = append(mutationTypes, mut)
	}
	sort.Strings(mutationTypes)

	// 写入表头
	header := "Sample,Total_Reads,Aligned_Reads,Good_Aligned_Reads,Reads_With_Mutations,Total_Mutations"
	if len(mutationTypes) > 0 {
		header += "," + strings.Join(mutationTypes, ",")
	}
	writer.WriteString(header + "\n")

	// 写入每行数据
	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		var row strings.Builder
		fmt.Fprintf(
			&row,
			"%s,%d,%d,%d,%d,%d",
			sampleName,
			sampleStats.ReadCounts,
			sampleStats.AlignedReads,
			sampleStats.GoodAlignedReads,
			sampleStats.SubstitutionReads,
			sampleStats.TotalSubstitution,
		)
		for _, mut := range mutationTypes {
			count := sampleStats.SubstitutionCount[mut]
			fmt.Fprintf(&row, ",%d", count)
		}
		writer.WriteString(row.String() + "\n")
		sampleStats.RUnlock()
	}

	writer.Flush()
	fmt.Printf("样本汇总已写入: %s\n", filename)

	// 2. 突变频谱（按频率排序）
	filename = filepath.Join(outputDir, "mutation_spectrum.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Mutation,Count,Frequency\n")

	// 创建突变类型和计数的切片用于排序
	type mutationCount struct {
		mutation string
		count    int
	}

	var mutationCounts []mutationCount
	for mut, count := range stats.SubstitutionCount {
		mutationCounts = append(mutationCounts, mutationCount{mut, count})
	}

	// 按计数降序排序
	sort.Slice(mutationCounts, func(i, j int) bool {
		return mutationCounts[i].count > mutationCounts[j].count
	})

	totalMutations := 0
	for _, count := range stats.SubstitutionCount {
		totalMutations += count
	}

	// 写入排序后的数据
	for _, mc := range mutationCounts {
		count := mc.count
		frequency := float64(count) / float64(totalMutations)
		percentage := frequency * 100
		fmt.Fprintf(writer, "%s,%d,%.6f,%.4f%%\n",
			mc.mutation, count, frequency, percentage)
	}
	writer.Flush()
	fmt.Printf("突变频谱已写入: %s\n", filename)

	// 3. 突变类型分析（transition/transversion等）
	filename = filepath.Join(outputDir, "mutation_analysis.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("MutationType,Count,Percentage\n")

	// 分析突变类型
	var transitions, transversions, other int
	for mut, count := range stats.SubstitutionCount {
		if len(mut) >= 3 && mut[1] == '>' {
			ref := mut[0]
			alt := mut[2]

			// 转换（嘌呤<->嘌呤或嘧啶<->嘧啶）
			if (ref == 'A' && alt == 'G') || (ref == 'G' && alt == 'A') ||
				(ref == 'C' && alt == 'T') || (ref == 'T' && alt == 'C') {
				transitions += count
			} else if (ref == 'A' && alt == 'C') || (ref == 'A' && alt == 'T') ||
				(ref == 'G' && alt == 'C') || (ref == 'G' && alt == 'T') ||
				(ref == 'C' && alt == 'A') || (ref == 'C' && alt == 'G') ||
				(ref == 'T' && alt == 'A') || (ref == 'T' && alt == 'G') {
				transversions += count
			} else {
				other += count
			}
		} else {
			other += count
		}
	}

	total := transitions + transversions + other
	if total > 0 {
		fmt.Fprintf(writer, "Transitions,%d,%.4f%%\n",
			transitions, float64(transitions)/float64(total)*100)
		fmt.Fprintf(writer, "Transversions,%d,%.4f%%\n",
			transversions, float64(transversions)/float64(total)*100)
		fmt.Fprintf(writer, "Other,%d,%.4f%%\n",
			other, float64(other)/float64(total)*100)
		fmt.Fprintf(writer, "Ti/Tv Ratio,%.4f,\n",
			float64(transitions)/float64(transversions))
	}

	writer.Flush()
	fmt.Printf("突变分析已写入: %s\n", filename)

	return nil
}

// writeReadTypeStats 写入read类型统计 - 更新输出
func writeReadTypeStats(stats *MutationStats, outputDir string) error {
	// 1. 各样本的read类型统计
	filename := filepath.Join(outputDir, "read_type_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 表头添加 GoodAlignedReads
	header := "Sample,TotalReads,AlignedReads,GoodAlignedReads,ReadsWithMutations"

	// 原有read类型列
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		header += "," + ReadTypeNames[rt]
	}

	// 新增：reads维度变异统计
	header += ",InsertReads,DeleteReads,SubstitutionReads"

	// 新增：变异事件个数维度
	header += ",InsertEventCount,DeleteEventCount,SubstitutionEventCount"

	// 新增：碱基个数维度
	header += ",InsertBaseTotal,DeleteBaseTotal,SubstitutionBaseTotal"

	writer.WriteString(header + "\n")

	// 写入每行数据
	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads
		goodAlignedReads := sampleStats.GoodAlignedReads
		readsWithMuts := sampleStats.SubstitutionReads

		row := fmt.Sprintf("%s,%d,%d,%d,%d", sampleName, totalReads, alignedReads, goodAlignedReads, readsWithMuts)

		// 写入每种read类型的计数
		for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
			count := sampleStats.ReadTypeCounts[rt]
			row += fmt.Sprintf(",%d", count)
		}

		// 新增：写入reads维度变异统计
		row += fmt.Sprintf(",%d,%d,%d",
			sampleStats.InsertReads,
			sampleStats.DeleteReads,
			sampleStats.SubstitutionReads)

		// 新增：写入变异事件个数维度
		row += fmt.Sprintf(",%d,%d,%d",
			sampleStats.InsertEventCount,
			sampleStats.DeleteEventCount,
			sampleStats.SubstitutionEventCount)

		// 新增：写入碱基个数维度
		row += fmt.Sprintf(",%d,%d,%d",
			sampleStats.InsertBaseTotal,
			sampleStats.DeleteBaseTotal,
			sampleStats.SubstitutionBaseTotal)

		sampleStats.RUnlock()

		writer.WriteString(row + "\n")
	}

	writer.Flush()
	fmt.Printf("样本read类型统计已写入: %s\n", filename)

	// 2. 汇总的read类型统计
	filename = filepath.Join(outputDir, "read_type_summary.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 写入表头 - 添加新统计
	writer.WriteString("Statistic,Count,PercentageOfGoodAlignedReads,PercentageOfAlignedReads,PercentageOfTotalReads\n")

	totalReads := stats.TotalReadCount
	alignedReads := stats.TotalAlignedReads
	readsCount := stats.TotalGoodAlignedReads
	baseCount := stats.TotalRefLengthGoodAligned
	totalVarReads := stats.TotalInsertReads + stats.TotalDeleteReads + stats.TotalSubstitutionReads
	totalEvents := stats.TotalInsertEventCount + stats.TotalDeleteEventCount + stats.TotalSubstitutionEventCount
	totalBases := stats.TotalInsertBaseTotal + stats.TotalDeleteBaseTotal + stats.TotalSubstitutionBaseTotal
	totalDel1 := lo.Sum(lo.Values(stats.Del1BaseCounts))

	// a. 原有read类型统计
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		write1line(writer, ReadTypeNames[rt], stats.ReadTypeCounts[rt], readsCount, alignedReads, totalReads)
	}

	writer.WriteString("\n变异大类统计（非互斥）:\n")
	// b. 新增：reads维度变异统计（非互斥）
	// writer.WriteString("\nReads维度变异统计（非互斥）:\n")
	// 包含插入的reads
	write1line(writer, "InsertReads", stats.TotalInsertReads, readsCount, baseCount, totalVarReads)
	// 包含缺失的reads
	write1line(writer, "DeleteReads", stats.TotalDeleteReads, readsCount, baseCount, totalVarReads)
	// 包含替换的reads
	write1line(writer, "SubstitutionReads", stats.TotalSubstitutionReads, readsCount, baseCount, totalVarReads)

	// c. 新增：变异事件个数维度
	// writer.WriteString("\n变异事件个数维度:\n")
	// 插入事件个数
	write1line(writer, "InsertEvents", stats.TotalInsertEventCount, readsCount, baseCount, totalEvents)
	// 缺失事件个数
	write1line(writer, "DeleteEvents", stats.TotalDeleteEventCount, readsCount, baseCount, totalEvents)
	// 替换事件个数
	write1line(writer, "SubstitutionEvents", stats.TotalSubstitutionEventCount, readsCount, baseCount, totalEvents)

	// d. 新增：碱基个数维度
	// writer.WriteString("\n碱基个数维度:\n")
	// 插入碱基总数
	write1line(writer, "InsertBases", stats.TotalInsertBaseTotal, readsCount, baseCount, totalBases)
	// 缺失碱基总数
	write1line(writer, "DeleteBases", stats.TotalDeleteBaseTotal, readsCount, baseCount, totalBases)
	// 替换碱基总数
	write1line(writer, "SubstitutionBases", stats.TotalSubstitutionBaseTotal, readsCount, baseCount, totalBases)

	// e. 汇总信息
	writer.WriteString("\n汇总信息:\n")
	write1line(writer, "TotalReads", totalReads, totalReads, totalReads, totalReads)
	write1line(writer, "AlignedReads", alignedReads, alignedReads, alignedReads, totalReads)
	write1line(writer, "GoodAlignedReads", readsCount, readsCount, alignedReads, totalReads)
	write1line(writer, "ReadsWithMutations", stats.TotalReadsWithMuts, readsCount, alignedReads, totalReads)

	// 替换个数分布
	writer.WriteString("\n替换个数分布:\n")
	var counts []int
	for c := range stats.SubstitutionCountDist {
		counts = append(counts, c)
	}
	sort.Ints(counts)
	for _, c := range counts {
		n := stats.SubstitutionCountDist[c]
		pct := float64(n) / float64(alignedReads) * 100
		fmt.Fprintf(writer, "%d,%d,%.4f%%\n", c, n, pct)
	}

	// 以追加方式或重新生成整个文件
	// 为了简洁，我们在原有writeReadTypeStats末尾添加新section
	// 假设我们已经写完了原有内容，现在追加
	writer.WriteString("\n细分类统计:\n")
	stats.WriteSubtype(writer)

	// ===== 新增：样品平均比例计算 =====
	sortedNames := stats.SampleNames // 使用已有的排序函数

	sums := make(map[string]*SumPct)

	// 初始化需要统计的项（与已有输出保持一致）
	// 主类型
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		sums[ReadTypeNames[rt]] = &SumPct{}
	}
	bases := []byte{'A', 'C', 'G', 'T'}
	for _, base := range bases {
		name := "Del1_" + string(base)
		sums[name] = &SumPct{}
	}

	// reads维度变异统计
	sums["InsertReads"] = &SumPct{}
	sums["DeleteReads"] = &SumPct{}
	sums["SubstitutionReads"] = &SumPct{}
	sums["ReadsWithMutations"] = &SumPct{}
	sums["GoodAlignedReads"] = &SumPct{}

	// 遍历每个样本，累加比例
	for _, sampleName := range sortedNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()
		good := sampleStats.GoodAlignedReads
		aligned := sampleStats.AlignedReads
		total := sampleStats.ReadCounts
		// 主类型比例
		for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
			count := sampleStats.ReadTypeCounts[rt]
			pctGood := float64(count) / float64(good) * 100
			pctAligned := float64(count) / float64(aligned) * 100
			pctTotal := float64(count) / float64(total) * 100
			sums[ReadTypeNames[rt]].SumGood += pctGood
			sums[ReadTypeNames[rt]].SumAligned += pctAligned
			sums[ReadTypeNames[rt]].SumTotal += pctTotal
			sums[ReadTypeNames[rt]].Count++
		}

		bases := []byte{'A', 'C', 'G', 'T'}
		for _, base := range bases {
			name := "Del1_" + string(base)
			sums[name].SumGood += float64(sampleStats.Del1BaseCounts[base]) / float64(good) * 100
			sums[name].SumAligned += float64(sampleStats.Del1BaseCounts[base]) / float64(aligned) * 100
			sums[name].SumTotal += float64(sampleStats.Del1BaseCounts[base]) / float64(total) * 100
			sums[name].Count++
		}

		// reads维度变异统计比例
		// InsertReads
		pctGood := float64(sampleStats.InsertReads) / float64(good) * 100
		pctAligned := float64(sampleStats.InsertReads) / float64(aligned) * 100
		pctTotal := float64(sampleStats.InsertReads) / float64(total) * 100
		sums["InsertReads"].SumGood += pctGood
		sums["InsertReads"].SumAligned += pctAligned
		sums["InsertReads"].SumTotal += pctTotal
		sums["InsertReads"].Count++

		// DeleteReads
		pctGood = float64(sampleStats.DeleteReads) / float64(good) * 100
		pctAligned = float64(sampleStats.DeleteReads) / float64(aligned) * 100
		pctTotal = float64(sampleStats.DeleteReads) / float64(total) * 100
		sums["DeleteReads"].SumGood += pctGood
		sums["DeleteReads"].SumAligned += pctAligned
		sums["DeleteReads"].SumTotal += pctTotal
		sums["DeleteReads"].Count++

		// SubstitutionReads
		pctGood = float64(sampleStats.SubstitutionReads) / float64(good) * 100
		pctAligned = float64(sampleStats.SubstitutionReads) / float64(aligned) * 100
		pctTotal = float64(sampleStats.SubstitutionReads) / float64(total) * 100
		sums["SubstitutionReads"].SumGood += pctGood
		sums["SubstitutionReads"].SumAligned += pctAligned
		sums["SubstitutionReads"].SumTotal += pctTotal
		sums["SubstitutionReads"].Count++

		// ReadsWithMutations
		pctGood = float64(sampleStats.SubstitutionReads) / float64(good) * 100
		pctAligned = float64(sampleStats.SubstitutionReads) / float64(aligned) * 100
		pctTotal = float64(sampleStats.SubstitutionReads) / float64(total) * 100
		sums["ReadsWithMutations"].SumGood += pctGood
		sums["ReadsWithMutations"].SumAligned += pctAligned
		sums["ReadsWithMutations"].SumTotal += pctTotal
		sums["ReadsWithMutations"].Count++

		// GoodAlignedReads
		pctGood = float64(sampleStats.GoodAlignedReads) / float64(good) * 100
		pctAligned = float64(sampleStats.GoodAlignedReads) / float64(aligned) * 100
		pctTotal = float64(sampleStats.GoodAlignedReads) / float64(total) * 100
		sums["GoodAlignedReads"].SumGood += pctGood
		sums["GoodAlignedReads"].SumAligned += pctAligned
		sums["GoodAlignedReads"].SumTotal += pctTotal
		sums["GoodAlignedReads"].Count++

		sampleStats.RUnlock()
	}

	// 写入样品平均比例部分
	writer.WriteString("\n样品平均比例（每个样本的比例取算术平均）:\n")
	writer.WriteString("Statistic,SampleMeanGoodAlignedPct,SampleMeanAlignedPct,SampleMeanTotalPct\n")

	// 定义输出顺序
	order := []string{
		ReadTypeNames[ReadTypeMatch],
		ReadTypeNames[ReadTypeInsert],
		ReadTypeNames[ReadTypeDelete],
		ReadTypeNames[ReadTypeSubstitution],
		ReadTypeNames[ReadTypeInsertDelete],
		ReadTypeNames[ReadTypeInsertSubstitution],
		ReadTypeNames[ReadTypeDeleteSubstitution],
		ReadTypeNames[ReadTypeAll],
		"InsertReads",
		"DeleteReads",
		"SubstitutionReads",
		"ReadsWithMutations",
		"GoodAlignedReads",
	}

	for _, name := range order {
		s := sums[name]
		if s == nil || s.Count == 0 {
			continue
		}
		meanGood := s.SumGood / float64(s.Count)
		meanAligned := s.SumAligned / float64(s.Count)
		meanTotal := s.SumTotal / float64(s.Count)
		fmt.Fprintf(writer, "%s,%.4f%%,%.4f%%,%.4f%%\n", name, meanGood, meanAligned, meanTotal)
	}

	// 在原有内容的合适位置（例如在“替换个数分布”之后）添加 Del1 碱基分布
	writer.WriteString("\nDel1缺失碱基分布:\n")
	for _, base := range bases {
		name := "Del1_" + string(base)
		write1line(writer, name, stats.Del1BaseCounts[base], readsCount, baseCount, totalDel1)
	}
	writer.WriteString("\nDel1缺失碱基分布(样品算术平均):\n")
	for _, base := range bases {
		name := "Del1_" + string(base)
		s := sums[name]
		if s == nil || s.Count == 0 {
			continue
		}
		meanGood := s.SumGood / float64(s.Count)
		meanAligned := s.SumAligned / float64(s.Count)
		meanTotal := s.SumTotal / float64(s.Count)
		fmt.Fprintf(writer, "%s_mean,%.4f,%.4f%%,%.4f%%,%.4f%%\n", name, s.SumGood, meanGood, meanAligned, meanTotal)
	}
	// 输出切除后参考序列 ACGT 组成及修正的 Del1 缺失率
	if stats.TotalRefLengthAfterTrim > 0 {
		writer.WriteString("\n切除后参考序列ACGT组成及Del1缺失率（按碱基归一化）:\n")
		totalBases := stats.TotalRefLengthAfterTrim
		acgtPct := make(map[byte]float64)
		var totalDel1Fix float64
		for _, base := range []byte{'A', 'C', 'G', 'T'} {
			cnt := stats.TotalRefACGTCounts[base]
			pct := float64(cnt) / float64(totalBases)
			totalDel1Fix += float64(stats.Del1BaseCounts[base]) / pct
			acgtPct[base] = pct
			fmt.Fprintf(writer, "%c_pct,%.4f%%\n", base, pct*100)
		}

		writer.WriteString("\nDel1缺失事件数及按碱基归一化率:\n")
		for _, base := range bases {
			name := "Del1_" + string(base)
			delCnt := stats.Del1BaseCounts[base] // 假设已统计
			readsRatio := float64(delCnt) * 100 / float64(readsCount) / acgtPct[base]
			baseRatio := float64(delCnt) * 100 / float64(baseCount) / acgtPct[base]
			ratio := float64(delCnt) * 100 / float64(totalDel1Fix) / acgtPct[base]
			fmt.Fprintf(writer, "%s_fix,%d,%.4f%%,%.4f%%,%.4f%%\n", name, delCnt, readsRatio, baseRatio, ratio)
		}
	} else {
		writer.WriteString("\n无参考序列信息，跳过ACGT组成分析\n")
	}

	writer.Flush()
	fmt.Printf("汇总read类型统计已写入: %s\n", filename)

	return nil
}

// writeDetailedStats 写入详细统计信息
func writeDetailedStats(stats *MutationStats, outputDir string) error {
	// 1. 插入长度分布
	filename := filepath.Join(outputDir, "insert_length_distribution.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("InsertLength,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	// 插入长度标签
	lengthLabels := map[int]string{
		1: "1",
		2: "2",
		3: "3",
		4: ">3",
	}

	for _, length := range []int{1, 2, 3, 4} {
		totalCount := stats.InsertLengthDist[length]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", lengthLabels[length]))

		// 各样本计数
		sampleCounts := []string{}
		// 各样本百分比（相对于比对reads）
		samplePercents := []string{}
		for sampleName, sampleStats := range stats.Samples {
			sampleStats.RLock()
			count := sampleStats.InsertLengthDist[length]
			sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))

			percent := 0.0
			if sampleStats.AlignedReads > 0 {
				percent = float64(count) / float64(sampleStats.AlignedReads) * 100
			}
			samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))

			sampleStats.RUnlock()
		}

		writer.WriteString(strings.Join(sampleCounts, ";") + ",")
		writer.WriteString(fmt.Sprintf("%d,", totalCount))

		writer.WriteString(strings.Join(samplePercents, ";") + ",")
		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("插入长度分布已写入: %s\n", filename)

	// 2. 缺失长度分布
	filename = filepath.Join(outputDir, "delete_length_distribution.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("DeleteLength,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	for _, length := range []int{1, 2, 3, 4} {
		totalCount := stats.DeleteLengthDist[length]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", lengthLabels[length]))

		// 各样本计数
		sampleCounts := []string{}
		// 各样本百分比（相对于比对reads）
		samplePercents := []string{}
		for sampleName, sampleStats := range stats.Samples {
			sampleStats.RLock()
			count := sampleStats.DeleteLengthDist[length]
			sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))

			percent := 0.0
			if sampleStats.AlignedReads > 0 {
				percent = float64(count) / float64(sampleStats.AlignedReads) * 100
			}
			samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))

			sampleStats.RUnlock()
		}

		writer.WriteString(strings.Join(sampleCounts, ";") + ",")
		writer.WriteString(fmt.Sprintf("%d,", totalCount))
		writer.WriteString(strings.Join(samplePercents, ";") + ",")
		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("缺失长度分布已写入: %s\n", filename)

	// 5. 比对率和变异比例（分母为比对reads）
	filename = filepath.Join(outputDir, "alignment_and_mutation_rates.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Sample,TotalReads,AlignedReads,AlignmentRate,ReadsWithMutations,MutationReadRate,TotalMutations,MutationRatePerRead\n")

	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads
		readsWithMuts := sampleStats.SubstitutionReads
		// 计算总突变数
		totalMutations := sampleStats.TotalSubstitution

		alignmentRate := 0.0
		mutationReadRate := 0.0
		mutationRatePerRead := 0.0

		if totalReads > 0 {
			alignmentRate = float64(alignedReads) / float64(totalReads) * 100
		}
		if alignedReads > 0 {
			mutationReadRate = float64(readsWithMuts) / float64(alignedReads) * 100
			mutationRatePerRead = float64(totalMutations) / float64(alignedReads)
		}

		writer.WriteString(fmt.Sprintf("%s,%d,%d,%.4f%%,%d,%.4f%%,%d,%.4f\n",
			sampleName, totalReads, alignedReads, alignmentRate,
			readsWithMuts, mutationReadRate, totalMutations, mutationRatePerRead))

		sampleStats.RUnlock()
	}

	// 总计行
	totalAlignmentRate := 0.0
	totalMutationReadRate := 0.0
	totalMutationRatePerRead := 0.0

	if stats.TotalReadCount > 0 {
		totalAlignmentRate = float64(stats.TotalAlignedReads) / float64(stats.TotalReadCount) * 100
	}
	totalMutations := 0
	for _, count := range stats.SubstitutionCount {
		totalMutations += count
	}
	if stats.TotalAlignedReads > 0 {
		totalMutationReadRate = float64(stats.TotalReadsWithMuts) / float64(stats.TotalAlignedReads) * 100

		totalMutationRatePerRead = float64(totalMutations) / float64(stats.TotalAlignedReads)
	}

	writer.WriteString(fmt.Sprintf("Total,%d,%d,%.4f%%,%d,%.4f%%,%d,%.4f\n",
		stats.TotalReadCount, stats.TotalAlignedReads, totalAlignmentRate,
		stats.TotalReadsWithMuts, totalMutationReadRate, totalMutations, totalMutationRatePerRead))

	writer.Flush()
	fmt.Printf("比对率和变异比例已写入: %s\n", filename)

	return nil
}

// writeBaseMutationStats 写入碱基维度的变异统计
func writeBaseMutationStats(stats *MutationStats, outputDir string) error {
	// 1. 碱基维度的变异类型统计
	filename := filepath.Join(outputDir, "base_mutation_counts.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Sample,TotalReads,AlignedReads,SubstitutionBases,InsertionBases,DeletionBases,SubstitutionRate,InsertionRate,DeletionRate\n")

	// 变异类型
	// mutationTypes := []string{"substitution", "insertion", "deletion"}

	// 总计行
	totalSubstitution := 0
	totalInsertion := 0
	totalDeletion := 0

	for _, sampleName := range stats.SampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads

		// 获取碱基计数
		substitutionBases := sampleStats.SubstitutionBaseTotal
		insertionBases := sampleStats.InsertBaseTotal
		deletionBases := sampleStats.DeleteBaseTotal

		totalSubstitution += substitutionBases
		totalInsertion += insertionBases
		totalDeletion += deletionBases

		// 计算比例（每千碱基）
		substitutionRate := 0.0
		insertionRate := 0.0
		deletionRate := 0.0

		if alignedReads > 0 {
			// 假设平均read长度，这里简化处理
			// 实际应该计算总比对碱基数
			substitutionRate = float64(substitutionBases) / float64(alignedReads) * 1000
			insertionRate = float64(insertionBases) / float64(alignedReads) * 1000
			deletionRate = float64(deletionBases) / float64(alignedReads) * 1000
		}

		fmt.Fprintf(writer, "%s,%d,%d,%d,%d,%d,%.4f,%.4f,%.4f\n",
			sampleName, totalReads, alignedReads,
			substitutionBases, insertionBases, deletionBases,
			substitutionRate, insertionRate, deletionRate)

		sampleStats.RUnlock()
	}

	totalSubstitutionRate := 0.0
	totalInsertionRate := 0.0
	totalDeletionRate := 0.0

	if stats.TotalAlignedReads > 0 {
		totalSubstitutionRate = float64(totalSubstitution) / float64(stats.TotalAlignedReads) * 1000
		totalInsertionRate = float64(totalInsertion) / float64(stats.TotalAlignedReads) * 1000
		totalDeletionRate = float64(totalDeletion) / float64(stats.TotalAlignedReads) * 1000
	}

	writer.WriteString(fmt.Sprintf("Total,%d,%d,%d,%d,%d,%.4f,%.4f,%.4f\n",
		stats.TotalReadCount, stats.TotalAlignedReads,
		totalSubstitution, totalInsertion, totalDeletion,
		totalSubstitutionRate, totalInsertionRate, totalDeletionRate))

	writer.Flush()
	fmt.Printf("碱基维度变异统计已写入: %s\n", filename)

	// 2. 单碱基缺失统计
	filename = filepath.Join(outputDir, "single_base_deletion_stats.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Base,SampleCounts,TotalCount,SampleReads,TotalReads,SampleAlignedReads,TotalAlignedReads\n")

	// 碱基顺序
	bases := []byte{'A', 'C', 'G', 'T', 'N'}

	for _, base := range bases {
		totalCount := stats.TotalDeleteBaseCounts[base]

		// 各样本计数
		sampleCounts := []string{}
		sampleReads := []string{}
		sampleAlignedReads := []string{}

		for _, sampleName := range stats.SampleNames {
			sampleStats, exists := stats.Samples[sampleName]
			if !exists {
				continue
			}
			sampleStats.RLock()

			count := sampleStats.Del1BaseCounts[base]
			if count > 0 {
				sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				sampleReads = append(sampleReads, fmt.Sprintf("%s:%d", sampleName, sampleStats.ReadCounts))
				sampleAlignedReads = append(sampleAlignedReads, fmt.Sprintf("%s:%d", sampleName, sampleStats.AlignedReads))
			}

			sampleStats.RUnlock()
		}

		writer.WriteString(fmt.Sprintf("%c,%s,%d,%s,%d,%s,%d\n",
			base,
			strings.Join(sampleCounts, ";"),
			totalCount,
			strings.Join(sampleReads, ";"),
			stats.TotalReadCount,
			strings.Join(sampleAlignedReads, ";"),
			stats.TotalAlignedReads))
	}

	writer.Flush()
	fmt.Printf("单碱基缺失统计已写入: %s\n", filename)

	// 3. 插入序列统计
	filename = filepath.Join(outputDir, "insertion_sequence_stats.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("InsertionSequence,SampleCounts,TotalCount\n")

	// 收集所有插入序列
	allInsertSeqs := make(map[string]bool)
	for seq := range stats.InsertBaseCounts {
		allInsertSeqs[seq] = true
	}

	// 转换为切片并按长度和字母排序
	var insertSeqs []string
	for seq := range allInsertSeqs {
		insertSeqs = append(insertSeqs, seq)
	}
	sort.Slice(insertSeqs, func(i, j int) bool {
		if len(insertSeqs[i]) != len(insertSeqs[j]) {
			return len(insertSeqs[i]) < len(insertSeqs[j])
		}
		return insertSeqs[i] < insertSeqs[j]
	})

	// 只输出前100个最常见的插入序列
	outputLimit := 100
	if len(insertSeqs) > outputLimit {
		insertSeqs = insertSeqs[:outputLimit]
	}

	for _, seq := range insertSeqs {
		totalCount := stats.InsertBaseCounts[seq]

		// 各样本计数
		sampleCounts := []string{}
		for _, sampleName := range stats.SampleNames {
			sampleStats, exists := stats.Samples[sampleName]
			if !exists {
				continue
			}
			sampleStats.RLock()

			counts := sampleStats.InsertBaseCounts
			if count, exists := counts[seq]; exists && count > 0 {
				sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
			}
			sampleStats.RUnlock()
		}

		writer.WriteString(fmt.Sprintf("%s,%s,%d\n",
			seq,
			strings.Join(sampleCounts, ";"),
			totalCount))
	}

	writer.Flush()
	fmt.Printf("插入序列统计已写入: %s\n", filename)

	return nil
}

// writeDeletionPositionStats 写入缺失位置统计
func writeDeletionPositionStats(stats *MutationStats, outputDir string) error {
	// 1. 各样本的单碱基缺失位置统计
	for sampleName, sampleStats := range stats.Samples {
		sampleStats.RLock()

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_single_base_deletion_positions.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}

		writer := bufio.NewWriter(file)
		writer.WriteString("Position,Base,Count,TotalReads,AlignedReads\n")

		// 收集位置信息并按位置排序
		type positionInfo struct {
			posKey string
			count  int
		}

		var positions []positionInfo
		for posKey, count := range sampleStats.DeletePositionCounts {
			positions = append(positions, positionInfo{posKey, count})
		}

		// 按位置排序
		sort.Slice(positions, func(i, j int) bool {
			// 解析位置
			posI := extractPosition(positions[i].posKey)
			posJ := extractPosition(positions[j].posKey)
			return posI < posJ
		})

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads

		for _, info := range positions {
			// 解析位置和碱基
			parts := strings.Split(info.posKey, ":")
			if len(parts) == 2 {
				position := parts[0]
				base := parts[1]
				writer.WriteString(fmt.Sprintf("%s,%s,%d,%d,%d\n",
					position, base, info.count, totalReads, alignedReads))
			}
		}

		writer.Flush()
		file.Close()
		sampleStats.RUnlock()
		slog.Debug("缺失位置统计已写入", "sampleName", sampleName, "filename", filename)
	}

	// 2. 汇总的单碱基缺失位置统计
	filename := filepath.Join(outputDir, "total_single_base_deletion_positions.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Position,Base,TotalCount,SampleCounts,SampleReads,SampleAlignedReads\n")

	// 收集所有位置信息
	type totalPositionInfo struct {
		posKey       string
		totalCount   int
		sampleCounts []string
	}

	var totalPositions []totalPositionInfo
	for posKey, totalCount := range stats.DeletePositionCounts {
		if totalCount > 0 {
			// 收集各样本的计数
			var sampleCounts []string
			for sampleName, sampleStats := range stats.Samples {
				sampleStats.RLock()

				if count, exists := sampleStats.DeletePositionCounts[posKey]; exists && count > 0 {
					sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				}

				sampleStats.RUnlock()
			}

			totalPositions = append(totalPositions, totalPositionInfo{
				posKey:       posKey,
				totalCount:   totalCount,
				sampleCounts: sampleCounts,
			})
		}
	}

	// 按总计数降序排序
	sort.Slice(totalPositions, func(i, j int) bool {
		if totalPositions[i].totalCount != totalPositions[j].totalCount {
			return totalPositions[i].totalCount > totalPositions[j].totalCount
		}
		return totalPositions[i].posKey < totalPositions[j].posKey
	})

	// 只输出前1000个最常见的位置
	outputLimit := 1000
	if len(totalPositions) > outputLimit {
		totalPositions = totalPositions[:outputLimit]
	}

	for _, info := range totalPositions {
		// 解析位置和碱基
		parts := strings.Split(info.posKey, ":")
		if len(parts) == 2 {
			position := parts[0]
			base := parts[1]

			// 收集样本reads信息
			var sampleReadsInfo []string
			var sampleAlignedReadsInfo []string

			for sampleName, sampleStats := range stats.Samples {
				sampleStats.RLock()

				if _, exists := sampleStats.DeletePositionCounts[info.posKey]; exists {
					sampleReadsInfo = append(sampleReadsInfo,
						fmt.Sprintf("%s:%d", sampleName, sampleStats.ReadCounts))
					sampleAlignedReadsInfo = append(sampleAlignedReadsInfo,
						fmt.Sprintf("%s:%d", sampleName, sampleStats.AlignedReads))
				}

				sampleStats.RUnlock()
			}

			writer.WriteString(fmt.Sprintf("%s,%s,%d,%s,%s,%s\n",
				position, base, info.totalCount,
				strings.Join(info.sampleCounts, ";"),
				strings.Join(sampleReadsInfo, ";"),
				strings.Join(sampleAlignedReadsInfo, ";")))
		}
	}

	writer.Flush()
	fmt.Printf("汇总缺失位置统计已写入: %s\n", filename)

	// 3. 缺失位置热力图数据
	filename = filepath.Join(outputDir, "deletion_position_heatmap.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 收集所有位置
	allPositions := make(map[int]bool)
	for posKey := range stats.DeletePositionCounts {
		if pos := extractPosition(posKey); pos > 0 {
			allPositions[pos] = true
		}
	}

	// 转换为排序的切片
	var positionsList []int
	for pos := range allPositions {
		positionsList = append(positionsList, pos)
	}
	sort.Ints(positionsList)

	// 写入表头
	header := "Position"
	bases := []string{"A", "C", "G", "T", "N"}
	for _, base := range bases {
		header += fmt.Sprintf(",Total_%s", base)
	}

	for _, sampleName := range stats.SampleNames {
		for _, base := range bases {
			header += fmt.Sprintf(",%s_%s", sampleName, base)
		}
	}

	writer.WriteString(header + "\n")

	// 写入每行数据
	for _, position := range positionsList {
		row := fmt.Sprintf("%d", position)

		// 总计数据
		for _, base := range bases {
			posKey := fmt.Sprintf("%d:%s", position, base)
			count := stats.DeletePositionCounts[posKey]
			row += fmt.Sprintf(",%d", count)
		}

		// 各样本数据
		for _, sampleName := range stats.SampleNames {
			sampleStats, exists := stats.Samples[sampleName]
			if exists {
				sampleStats.RLock()
			}

			for _, base := range bases {
				posKey := fmt.Sprintf("%d:%s", position, base)
				count := 0
				if exists {
					count = sampleStats.DeletePositionCounts[posKey]
				}
				row += fmt.Sprintf(",%d", count)
			}

			if exists {
				sampleStats.RUnlock()
			}
		}

		writer.WriteString(row + "\n")
	}

	writer.Flush()
	fmt.Printf("缺失位置热力图数据已写入: %s\n", filename)

	return nil
}

// extractPosition 从位置键中提取位置数字
func extractPosition(posKey string) int {
	parts := strings.Split(posKey, ":")
	if len(parts) >= 1 {
		pos, err := strconv.Atoi(parts[0])
		if err == nil {
			return pos
		}
	}
	return 0
}

func (stats *MutationStats) MainWrite() {
	outputDir := stats.BatchInfo.OutputDir
	// 写入各bam各位置各碱基变化组合的个数分布统计
	if err := writePositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入位置统计失败: %v\n", err)
	}

	// 写入各bam各碱基变化组合的分布
	if err := writeSampleMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入样本突变统计失败: %v\n", err)
	}

	// 写入总碱基变化组合的分布
	if err := writeTotalMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入总突变统计失败: %v\n", err)
	}

	// 写入read类型统计
	if err := writeReadTypeStats(stats, outputDir); err != nil {
		fmt.Printf("写入read类型统计失败: %v\n", err)
	}
	// 写入汇总报告
	if err := writeSummaryReport(stats, outputDir); err != nil {
		fmt.Printf("写入汇总报告失败: %v\n", err)
	}

	// 新增：写入详细统计
	if err := writeDetailedStats(stats, outputDir); err != nil {
		fmt.Printf("写入详细统计失败: %v\n", err)
	}

	// 新增：写入碱基维度统计
	if err := writeBaseMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入碱基维度统计失败: %v\n", err)
	}

	// 新增：写入缺失位置统计
	if err := writeDeletionPositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入缺失位置统计失败: %v\n", err)
	}

	// 新增：细分类统计输出
	if err := writeReadSubtypeStats(stats, outputDir); err != nil {
		fmt.Printf("写入细分类统计失败: %v\n", err)
	}

	// 新增：样品-位置详细统计
	if err := writePositionDetailedStats(stats, outputDir); err != nil {
		fmt.Printf("写入位置详细统计失败: %v\n", err)
	}

	if err := writeNMerStats(stats, outputDir); err != nil {
		fmt.Printf("写入 N-mer 统计失败: %v\n", err)
	}

	if err := writeSubstitutionStats(stats, outputDir); err != nil {
		fmt.Printf("写入替换突变统计失败: %v\n", err)
	}
}

func (stats *MutationStats) MainPrint() {
	// 打印总体统计
	fmt.Printf("\n处理完成!\n")
	fmt.Printf("总体统计:\n")
	fmt.Printf("  总reads数: %d\n", stats.TotalReadCount)
	fmt.Printf("  细分类统计:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.ReadTypeCounts[rt]
		if count > 0 {
			percentage := float64(count) / float64(stats.TotalReadCount) * 100
			fmt.Printf("    %s: %d (%.2f%%)\n", ReadTypeNames[rt], count, percentage)
		}
	}
	stats.WriteSubtype(os.Stdout)
}

// 新增：输出细分类统计
func writeReadSubtypeStats(stats *MutationStats, outputDir string) error {
	// 1. 样本维度细分类统计（每个样本一行，每种细分类一列？为了可读性，转置输出：每行是一个样本+细分类）
	filename := filepath.Join(outputDir, "read_subtype_stats.csv")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Sample,Category,Subtype,Reads,Events,Bases,AlignedReads,TotalReads\n")

	// 获取排序后的样本名列表
	sampleNames := stats.SampleNames

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()

		aligned := sampleStats.AlignedReads
		total := sampleStats.ReadCounts

		// 缺失细分类
		for st, cnt := range sampleStats.DeleteSubtypeReads {
			fmt.Fprintf(writer, "%s,Deletion,%s,%d,%d,%d,%d,%d\n",
				sampleName, DeleteNames[st], cnt,
				sampleStats.DeleteSubtypeEvents[st],
				sampleStats.DeleteSubtypeBases[st],
				aligned, total,
			)
		}
		// 插入细分类
		for st, cnt := range sampleStats.InsertSubtypeReads {
			fmt.Fprintf(writer, "%s,Insertion,%s,%d,%d,%d,%d,%d\n",
				sampleName, InsertNames[st], cnt,
				sampleStats.InsertSubtypeEvents[st],
				sampleStats.InsertSubtypeBases[st],
				aligned, total,
			)
		}
		for st, cnt := range sampleStats.SubstitutionSubtypeReads {
			fmt.Fprintf(writer, "%s,Substitution,%s,%d,%d,%d,%d,%d\n",
				sampleName, SubstNames[st], cnt,
				sampleStats.SubstitutionSubtypeEvents[st],
				sampleStats.SubstitutionSubtypeBases[st],
				aligned, total,
			)
		}

		sampleStats.RUnlock()
	}

	// 汇总行
	writer.WriteString("Total,,,,,\n")
	// 缺失汇总
	for st, cnt := range stats.DeleteSubtypeReads {
		var subtypeName = DeleteNames[st]
		fmt.Fprintf(writer, "Total,Deletion,%s,%d,%d,%d,%d,%d\n",
			subtypeName, cnt,
			stats.DeleteSubtypeEvents[st],
			stats.DeleteSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount)
	}
	// 插入汇总
	for st, cnt := range stats.InsertSubtypeReads {
		fmt.Fprintf(writer, "Total,Insertion,%s,%d,%d,%d,%d,%d\n",
			InsertNames[st], cnt,
			stats.InsertSubtypeEvents[st],
			stats.InsertSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount)
	}
	// 替换汇总
	for st, cnt := range stats.SubstitutionSubtypeReads {
		fmt.Fprintf(writer, "Total,Substitution,%s,%d,%d,%d,%d,%d\n",
			SubstNames[st], cnt,
			stats.SubstitutionSubtypeEvents[st],
			stats.SubstitutionSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount)
	}

	// 新增：输出 Del3 前一个碱基和第一个碱基分布
	// 定义碱基顺序
	bases := []byte{'A', 'C', 'G', 'T', 'N'}

	// 样品维度
	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()
		aligned := sampleStats.AlignedReads
		total := sampleStats.ReadCounts
		refLen := sampleStats.RefLengthAfterTrim
		refCounts := sampleStats.RefACGTCounts

		// Del3PrevBase
		for _, base := range bases {
			cnt := sampleStats.Del3PrevBaseCounts[base]
			if cnt > 0 {
				writer.WriteString(fmt.Sprintf("%s,Del3PrevBase,%c,%d,%d,%d,%d,%d\n",
					sampleName, base, 0, cnt, 0, aligned, total)) // Reads=0, Events=cnt, Bases=0
			}
		}
		// Del3FirstBase
		for _, base := range bases {
			cnt := sampleStats.Del3FirstBaseCounts[base]
			if cnt > 0 {
				writer.WriteString(fmt.Sprintf("%s,Del3FirstBase,%c,%d,%d,%d,%d,%d\n",
					sampleName, base, 0, cnt, 0, aligned, total))
			}
		}

		// Del3PrevBase_fix
		for _, base := range bases {
			cnt := sampleStats.Del3PrevBaseCounts[base]
			if cnt > 0 && refLen > 0 {
				baseCnt := refCounts[base]
				if baseCnt > 0 {
					adjusted := float64(cnt) * float64(refLen) / float64(baseCnt)
					writer.WriteString(fmt.Sprintf("%s,Del3PrevBase_fix,%c,%d,%.4f,%d,%d,%d\n",
						sampleName, base, 0, adjusted, 0, aligned, total))
				}
			}
		}
		// Del3FirstBase_fix
		for _, base := range bases {
			cnt := sampleStats.Del3FirstBaseCounts[base]
			if cnt > 0 && refLen > 0 {
				baseCnt := refCounts[base]
				if baseCnt > 0 {
					adjusted := float64(cnt) * float64(refLen) / float64(baseCnt)
					writer.WriteString(fmt.Sprintf("%s,Del3FirstBase_fix,%c,%d,%.4f,%d,%d,%d\n",
						sampleName, base, 0, adjusted, 0, aligned, total))
				}
			}
		}
		// 遍历所有可能的组合
		for _, b1 := range bases {
			for _, b2 := range bases {
				key := fmt.Sprintf("%c>%c", b1, b2)
				cnt := sampleStats.Del3PrevFirstCombCounts[key]
				if cnt == 0 {
					continue
				}
				// 原始计数
				writer.WriteString(fmt.Sprintf("%s,Del3PrevFirstComb,%s,%d,%d,%d,%d,%d\n",
					sampleName, key, 0, cnt, 0, aligned, total))
				// 修正值（按碱基比例）
				if refLen > 0 {
					base1Cnt := refCounts[b1]
					base2Cnt := refCounts[b2]
					if base1Cnt > 0 && base2Cnt > 0 {
						// 归一化因子：期望计数 = (base1Cnt/refLen)*(base2Cnt/refLen)*refLen = base1Cnt*base2Cnt/refLen
						// 调整为每参考碱基的计数，乘以refLen保持量纲
						expected := float64(base1Cnt*base2Cnt) / float64(refLen)
						adjusted := float64(cnt) / expected // 实际/期望
						// 或者乘以refLen得到每参考长度的计数：adjusted = float64(cnt) * float64(refLen) / float64(base1Cnt*base2Cnt)
						// 我们采用后者，与单个碱基的修正公式一致（单个碱基修正：cnt * refLen / baseCnt）
						adjusted = float64(cnt) * float64(refLen) / float64(base1Cnt*base2Cnt)
						writer.WriteString(fmt.Sprintf("%s,Del3PrevFirstComb_fix,%s,%d,%.4f,%d,%d,%d\n",
							sampleName, key, 0, adjusted, 0, aligned, total))
					}
				}
			}
		}
		sampleStats.RUnlock()
	}

	// 汇总行
	writer.WriteString("Total,Del3PrevBase,,\n") // 占位
	for _, base := range bases {
		cnt := stats.Del3PrevBaseCounts[base]
		if cnt > 0 {
			fmt.Fprintf(writer, "Total,Del3PrevBase,%c,%d,%d,%d,%d,%d\n",
				base, 0, cnt, 0, stats.TotalAlignedReads, stats.TotalReadCount)
		}
	}
	writer.WriteString("Total,Del3FirstBase,,\n")
	for _, base := range bases {
		cnt := stats.Del3FirstBaseCounts[base]
		if cnt > 0 {
			fmt.Fprintf(writer, "Total,Del3FirstBase,%c,%d,%d,%d,%d,%d\n",
				base, 0, cnt, 0, stats.TotalAlignedReads, stats.TotalReadCount)
		}
	}
	// 总维度
	totalRefLen := stats.TotalRefLengthAfterTrim
	totalRefCounts := stats.TotalRefACGTCounts
	for _, b1 := range bases {
		for _, b2 := range bases {
			key := fmt.Sprintf("%c>%c", b1, b2)
			cnt := stats.Del3PrevFirstCombCounts[key]
			if cnt == 0 {
				continue
			}
			// 原始计数
			fmt.Fprintf(writer, "Total,Del3PrevFirstComb,%s,%d,%d,%d,%d,%d\n",
				key, 0, cnt, 0, stats.TotalAlignedReads, stats.TotalReadCount)
			// 修正值
			if totalRefLen > 0 {
				base1Cnt := totalRefCounts[b1]
				base2Cnt := totalRefCounts[b2]
				if base1Cnt > 0 && base2Cnt > 0 {
					adjusted := float64(cnt) * float64(totalRefLen) / float64(base1Cnt*base2Cnt)
					writer.WriteString(fmt.Sprintf("Total,Del3PrevFirstComb_fix,%s,%d,%.4f,%d,%d,%d\n",
						key, 0, adjusted, 0, stats.TotalAlignedReads, stats.TotalReadCount))
				}
			}
		}
	}

	// 总维度
	for _, base := range bases {
		cnt := stats.Del3PrevBaseCounts[base]
		if cnt > 0 && totalRefLen > 0 {
			baseCnt := totalRefCounts[base]
			if baseCnt > 0 {
				adjusted := float64(cnt) * float64(totalRefLen) / float64(baseCnt)
				fmt.Fprintf(writer, "Total,Del3PrevBase_fix,%c,%d,%.4f,%d,%d,%d\n",
					base, 0, adjusted, 0, stats.TotalAlignedReads, stats.TotalReadCount)
			}
		}
	}
	for _, base := range bases {
		cnt := stats.Del3FirstBaseCounts[base]
		if cnt > 0 && totalRefLen > 0 {
			baseCnt := totalRefCounts[base]
			if baseCnt > 0 {
				adjusted := float64(cnt) * float64(totalRefLen) / float64(baseCnt)
				fmt.Fprintf(writer, "Total,Del3FirstBase_fix,%c,%d,%.4f,%d,%d,%d\n",
					base, 0, adjusted, 0, stats.TotalAlignedReads, stats.TotalReadCount)
			}
		}
	}

	writer.Flush()
	fmt.Printf("细分类统计已写入: %s\n", filename)

	// 2. 细分类组合统计
	filename = filepath.Join(outputDir, "read_subtype_combinations.csv")
	file, err = os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Combination,Sample,Count,AlignedReads,TotalReads\n")

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()
		aligned := sampleStats.AlignedReads
		total := sampleStats.ReadCounts
		for comb, cnt := range sampleStats.SubtypeCombinationCounts {
			fmt.Fprintf(writer, "%s,%s,%d,%d,%d\n",
				comb, sampleName, cnt, aligned, total)
		}
		sampleStats.RUnlock()
	}

	// 汇总组合统计
	for comb, cnt := range stats.SubtypeCombinationCounts {
		fmt.Fprintf(writer, "%s,Total,%d,%d,%d\n",
			comb, cnt, stats.TotalAlignedReads, stats.TotalReadCount)
	}

	writer.Flush()
	fmt.Printf("细分类组合统计已写入: %s\n", filename)

	return nil
}

// ========== 新增输出函数：样品-位置详细统计 ==========
func writePositionDetailedStats(stats *MutationStats, outputDir string) error {
	sampleNames := stats.SampleNames

	// 汇总数据结构：修正后位置 -> TotalPositionDetail
	totalPosStats := make(map[int]*PositionDetail)

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()

		refLength := sampleStats.Sample.RefLength
		headCut := sampleStats.Sample.HeadCut
		tailCut := sampleStats.Sample.TailCut
		// 计算有效区间（1-based）
		minPos := 1
		maxPos := int(^uint(0) >> 1)
		if refLength > 0 {
			minPos = headCut + 1
			maxPos = refLength - tailCut
		}

		// 样品输出文件
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_position_detailed.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			sampleStats.RUnlock()
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		writer := bufio.NewWriter(file)
		// 写入表头：不再包含百分比，改为计数
		writer.WriteString("pos,base,depth,match_pure,match_with_ins,mismatch_pure,mismatch_with_ins,insertion,deletion,del1,perfect_reads,perfect_upto_pos\n")

		var positions []int
		for pos := range sampleStats.PositionStats {
			positions = append(positions, pos)
		}
		sort.Ints(positions)

		for _, rawPos := range positions {
			if rawPos < minPos || rawPos > maxPos {
				continue
			}
			detail := sampleStats.PositionStats[rawPos]
			correctedPos := rawPos - headCut // 新位置从1开始

			// 样品行输出计数（不计算百分比）
			writer.WriteString(
				fmt.Sprintf("%d,%c,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
					correctedPos, detail.Base, detail.Depth,
					detail.MatchPure, detail.MatchWithIns,
					detail.MismatchPure, detail.MismatchWithIns,
					detail.Insertion, detail.Deletion, detail.Del1,
					detail.PerfectReadsCount, detail.PerfectUptoPosCount,
				),
			)

			// 累加到汇总
			total, ok := totalPosStats[correctedPos]
			if !ok {
				total = &PositionDetail{}
				totalPosStats[correctedPos] = total
			}
			total.Depth += detail.Depth
			total.MatchPure += detail.MatchPure
			total.MatchWithIns += detail.MatchWithIns
			total.MismatchPure += detail.MismatchPure
			total.MismatchWithIns += detail.MismatchWithIns
			total.Insertion += detail.Insertion
			total.Deletion += detail.Deletion
			total.Del1 += detail.Del1
			total.PerfectReadsCount += detail.PerfectReadsCount
			total.PerfectUptoPosCount += detail.PerfectUptoPosCount
			// 累加该样品的 aligned 和 total
			total.AlignedSum += sampleStats.AlignedReads
			total.TotalSum += sampleStats.ReadCounts
		}

		writer.Flush()
		file.Close()
		sampleStats.RUnlock()
		slog.Debug("已写入", "filename", filename)

	}

	// 写入汇总文件
	filename := filepath.Join(outputDir, "total_position_detailed.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建汇总文件失败: %v", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString("pos,depth,match_pure,match_with_ins,mismatch_pure,mismatch_with_ins,insertion,deletion,del1,perfect_reads,perfect_upto_pos,aligned,total\n")

	var totalPositions []int
	for pos := range totalPosStats {
		totalPositions = append(totalPositions, pos)
	}
	sort.Ints(totalPositions)

	for _, pos := range totalPositions {
		detail := totalPosStats[pos]
		writer.WriteString(
			fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
				pos, detail.Depth,
				detail.MatchPure, detail.MatchWithIns,
				detail.MismatchPure, detail.MismatchWithIns,
				detail.Insertion, detail.Deletion, detail.Del1,
				detail.PerfectReadsCount, detail.PerfectUptoPosCount,
				detail.AlignedSum, detail.TotalSum,
			),
		)
	}
	writer.Flush()
	slog.Debug("  已写入", "filename", filename)

	return nil
}

func writeNMerStats(stats *MutationStats, outputDir string) error {
	sampleNames := stats.SampleNames
	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()

		// 使用完整的参考序列
		fullSeq := sampleStats.Sample.RefSeqFull
		/* 		if fullSeq == "" {
			fmt.Printf("  警告: 样本 %s 无参考序列，跳过 N-mer 统计\n", sampleName)
			sampleStats.RUnlock()
			continue
		} */

		// 确定有效位置范围（切除头尾后）
		refLen := sampleStats.Sample.RefLength
		headCut := sampleStats.Sample.HeadCut
		tailCut := sampleStats.Sample.TailCut
		minPos := 1
		maxPos := int(^uint(0) >> 1)
		if refLen > 0 {
			minPos = headCut + 1
			maxPos = refLen - tailCut
		}

		// 收集并排序位置
		var positions []int
		for pos := range sampleStats.PositionStats {
			if pos >= minPos && pos <= maxPos {
				positions = append(positions, pos)
			}
		}
		sort.Ints(positions)

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_nmer_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			sampleStats.RUnlock()
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		writer := bufio.NewWriter(file)
		writer.WriteString("pos,prefix_N,base,N_correct,N+1_correct,step_accuracy\n")

		for _, rawPos := range positions {
			correctedPos := rawPos - headCut // 切除头后的新位置
			prefix := "NNNN"
			base := "N"

			if rawPos > stats.BatchInfo.NMerSize {
				// 提取前缀：原始序列中 [rawPos-N, rawPos-1] 区间
				startIdx := rawPos - stats.BatchInfo.NMerSize - 1 // 0-based 起始
				endIdx := rawPos - 2                              // 0-based 结束（包含）
				if len(fullSeq) >= rawPos {
					prefix = fullSeq[startIdx : endIdx+1]
					base = fullSeq[rawPos-1 : rawPos] // 当前碱基
				}
			}

			ps := sampleStats.PositionStats[rawPos]
			if ps == nil {
				continue
			}
			stepAcc := 0.0
			if ps.NCorrect > 0 {
				stepAcc = float64(ps.N1Correct) / float64(ps.NCorrect)
			}
			fmt.Fprintf(writer, "%d,%s,%s,%d,%d,%.6f\n",
				correctedPos, prefix, base, ps.NCorrect, ps.N1Correct, stepAcc)
		}

		writer.Flush()
		file.Close()
		sampleStats.RUnlock()
		slog.Debug("  已写入", "filename", filename)
	}
	return nil
}

// writeSubstitutionStats 输出替换突变的样品维度和总体统计
func writeSubstitutionStats(stats *MutationStats, outputDir string) error {
	sampleNames := stats.SampleNames

	// 收集所有突变类型（从所有样本中）
	allMuts := make(map[string]bool)
	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()
		for mut := range sampleStats.SubstitutionCount {
			allMuts[mut] = true
		}
		sampleStats.RUnlock()
	}
	var mutList []string
	for mut := range allMuts {
		mutList = append(mutList, mut)
	}
	sort.Strings(mutList)

	// 准备数据：每个样本的替换突变计数和总替换数
	type sampleData struct {
		totalSubs    int
		subCounts    map[string]int
		alignedBases int
		refBaseCnt   map[byte]int // 参考序列中ACGT的计数
		refBase      int          // 参考序列碱基数
	}
	sampleDataMap := make(map[string]*sampleData)
	// totalAlignedBases := stats.TotalAlignedBases
	totalSubsAll := 0
	totalRefBaseCntAll := map[byte]int{'A': 0, 'C': 0, 'G': 0, 'T': 0, 'N': 0}
	totalRefBase := 0

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()
		data := &sampleData{
			totalSubs:    0,
			subCounts:    make(map[string]int),
			alignedBases: sampleStats.AlignedBases,
			refBaseCnt:   make(map[byte]int),
		}
		// 复制 refACGTCounts
		for b, c := range sampleStats.RefACGTCounts {
			data.refBaseCnt[b] = c
			data.refBase += c
			totalRefBaseCntAll[b] += c
			totalRefBase += c

		}
		for mut, cnt := range sampleStats.SubstitutionCount {
			// 只统计替换（突变类型长度>2且中间是>，且 ref/alt 都是ACGTN，其实所有 mutation 都是替换）
			if len(mut) == 3 && mut[1] == '>' {
				data.subCounts[mut] = cnt
				data.totalSubs += cnt
				totalSubsAll += cnt
			}
		}
		sampleDataMap[sampleName] = data
		sampleStats.RUnlock()
	}

	// 输出文件
	filename := filepath.Join(outputDir, "substitution_stats.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString("ID,Mutation,Count,Frequency,Ratio,RatioFix\n")

	// 样品维度
	for _, sampleName := range sampleNames {
		data := sampleDataMap[sampleName]
		if data == nil {
			continue
		}
		for _, mut := range mutList {
			cnt := data.subCounts[mut]
			if cnt == 0 {
				continue
			}
			// Frequency = cnt / data.totalSubs
			freq := 0.0
			// if data.totalSubs > 0 {
			freq = float64(cnt) / float64(data.totalSubs)
			// }
			// Ratio = cnt / data.alignedBases
			ratio := 0.0
			// if data.alignedBases > 0 {
			ratio = float64(cnt) / float64(data.alignedBases)
			// }
			// RatioFix = cnt / refBaseCnt[refBase]
			refBase := mut[0] // 第一个字符
			refCnt := data.refBaseCnt[refBase]
			ratioFix := 0.0
			// if refCnt > 0 {
			ratioFix = float64(cnt) / float64(data.alignedBases) / float64(refCnt) * float64(data.refBase)
			// }
			fmt.Fprintf(writer, "%s,%s,%d,%.6g,%.6g,%.6g\n",
				sampleName, mut, cnt, freq, ratio, ratioFix)
		}
	}

	// 总体：加权平均（即直接使用总计数计算）
	// 计算总体的总替换数、总比对碱基、总参考碱基数
	totalSubs := totalSubsAll
	totalAligned := stats.TotalAlignedBases
	// 总参考碱基计数已累计在 totalRefBaseCntAll 中

	for _, mut := range mutList {
		// 先统计所有样本中该突变的计数总和
		totalCnt := 0
		for _, data := range sampleDataMap {
			totalCnt += data.subCounts[mut]
		}
		if totalCnt == 0 {
			continue
		}
		// Frequency 加权
		freq := 0.0
		// if totalSubs > 0 {
		freq = float64(totalCnt) / float64(totalSubs)
		// }
		// Ratio 加权
		ratio := 0.0
		// if totalAligned > 0 {
		ratio = float64(totalCnt) / float64(totalAligned)
		// }
		// RatioFix 加权
		refBase := mut[0]
		refCnt := totalRefBaseCntAll[refBase]
		ratioFix := 0.0
		// if refCnt > 0 {
		ratioFix = float64(totalCnt) / float64(totalAligned) / float64(refCnt) * float64(totalRefBase)
		// }
		fmt.Fprintf(writer, "WeightedAverage,%s,%d,%.6g,%.6g,%.6g\n",
			mut, totalCnt, freq, ratio, ratioFix)
	}

	// 算术平均：对所有样品的比值求算术平均
	// 需要先计算每个样品的每个突变的三种比值，然后对每个突变求平均
	for _, mut := range mutList {
		sumFreq := 0.0
		sumRatio := 0.0
		sumRatioFix := 0.0
		nSamples := 0
		for _, sampleName := range sampleNames {
			data := sampleDataMap[sampleName]
			cnt := data.subCounts[mut]
			if cnt == 0 {
				continue
			}
			nSamples++
			// Frequency 样品内
			// if data.totalSubs > 0 {
			sumFreq += float64(cnt) / float64(data.totalSubs)
			// }
			// Ratio 样品内
			// if data.alignedBases > 0 {
			sumRatio += float64(cnt) / float64(data.alignedBases)
			// }
			// RatioFix 样品内
			refBase := mut[0]
			// if data.refBaseCnt[refBase] > 0 {
			sumRatioFix += float64(cnt) / float64(data.alignedBases) / float64(data.refBaseCnt[refBase]) * float64(data.refBase)
			// }
		}
		if nSamples == 0 {
			continue
		}
		avgFreq := sumFreq / float64(nSamples)
		avgRatio := sumRatio / float64(nSamples)
		avgRatioFix := sumRatioFix / float64(nSamples)
		// 对于算术平均，Count 列可以填该突变的总数（或留空？），这里我们填总数以保持一致性
		totalCnt := 0
		for _, data := range sampleDataMap {
			totalCnt += data.subCounts[mut]
		}
		fmt.Fprintf(writer, "ArithmeticAverage,%s,%d,%.6g,%.6g,%.6g\n",
			mut, totalCnt, avgFreq, avgRatio, avgRatioFix)
	}

	writer.Flush()
	fmt.Printf("替换突变统计已写入: %s\n", filename)
	return nil
}
