package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 更新输出函数以使用新的数据结构

// writePositionStats 写入各bam各位置各碱基变化组合的个数分布统计
func writePositionStats(stats *MutationStats, outputDir string) error {
	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
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
					sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.ReadsWithMutations,
				)
				count2 += count
			}
			fmt.Fprintf(writer2, "%s,%s,%d,%d,%d,%d\n",
				sampleName, pos, count2,
				sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.ReadsWithMutations,
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
	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
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
		for mut := range sampleStats.Mutations {
			mutations = append(mutations, mut)
		}
		sort.Strings(mutations)

		for _, mut := range mutations {
			count := sampleStats.Mutations[mut]
			fmt.Fprintf(writer, "%s,%s,%d,%d,%d,%d\n",
				sampleName, mut, count,
				sampleStats.ReadCounts, sampleStats.AlignedReads, sampleStats.ReadsWithMutations,
			)
		}

		writer.Flush()
		sampleStats.RUnlock()
		fmt.Printf("  已写入: %s\n", filename)
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
	for mut := range stats.TotalMutations {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	// 获取所有样本的总reads数和包含突变reads数的和
	totalReads := stats.TotalReadCount
	totalReadsWithMuts := stats.TotalReadsWithMuts

	for _, mut := range mutations {
		count := stats.TotalMutations[mut]
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
		for mut := range sampleStats.Mutations {
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

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
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
			sampleStats.ReadsWithMutations,
			sampleStats.TotalMutations,
		)
		for _, mut := range mutationTypes {
			count := sampleStats.Mutations[mut]
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
	for mut, count := range stats.TotalMutations {
		mutationCounts = append(mutationCounts, mutationCount{mut, count})
	}

	// 按计数降序排序
	sort.Slice(mutationCounts, func(i, j int) bool {
		return mutationCounts[i].count > mutationCounts[j].count
	})

	totalMutations := 0
	for _, count := range stats.TotalMutations {
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
	for mut, count := range stats.TotalMutations {
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

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads
		goodAlignedReads := sampleStats.GoodAlignedReads
		readsWithMuts := sampleStats.ReadsWithMutations

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
	writer.WriteString("Statistic,Count,PercentageOfAlignedReads,PercentageOfTotalReads\n")

	totalReads := stats.TotalReadCount
	alignedReads := stats.TotalAlignedReads

	// a. 原有read类型统计
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		percentageAligned := 0.0
		percentageTotal := 0.0

		if alignedReads > 0 {
			percentageAligned = float64(count) / float64(alignedReads) * 100
		}
		if totalReads > 0 {
			percentageTotal = float64(count) / float64(totalReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%,%.4f%%\n",
			ReadTypeNames[rt], count, percentageAligned, percentageTotal))
	}

	// b. 新增：reads维度变异统计（非互斥）
	writer.WriteString("\nReads维度变异统计（非互斥）:\n")

	// 包含插入的reads
	percentageAligned := 0.0
	percentageTotal := 0.0
	if alignedReads > 0 {
		percentageAligned = float64(stats.TotalInsertReads) / float64(alignedReads) * 100
	}
	if totalReads > 0 {
		percentageTotal = float64(stats.TotalInsertReads) / float64(totalReads) * 100
	}
	writer.WriteString(fmt.Sprintf("InsertReads,%d,%.4f%%,%.4f%%\n",
		stats.TotalInsertReads, percentageAligned, percentageTotal))

	// 包含缺失的reads
	percentageAligned = 0.0
	percentageTotal = 0.0
	if alignedReads > 0 {
		percentageAligned = float64(stats.TotalDeleteReads) / float64(alignedReads) * 100
	}
	if totalReads > 0 {
		percentageTotal = float64(stats.TotalDeleteReads) / float64(totalReads) * 100
	}
	writer.WriteString(fmt.Sprintf("DeleteReads,%d,%.4f%%,%.4f%%\n",
		stats.TotalDeleteReads, percentageAligned, percentageTotal))

	// 包含替换的reads
	percentageAligned = 0.0
	percentageTotal = 0.0
	if alignedReads > 0 {
		percentageAligned = float64(stats.TotalSubstitutionReads) / float64(alignedReads) * 100
	}
	if totalReads > 0 {
		percentageTotal = float64(stats.TotalSubstitutionReads) / float64(totalReads) * 100
	}
	writer.WriteString(fmt.Sprintf("SubstitutionReads,%d,%.4f%%,%.4f%%\n",
		stats.TotalSubstitutionReads, percentageAligned, percentageTotal))

	// 在原有“reads维度变异统计”部分之后，添加基于良好reads的比例
	writer.WriteString("\n基于良好reads的统计（替换个数 <= 阈值）:\n")
	goodAligned := stats.TotalGoodAlignedReads
	if goodAligned > 0 {
		// 例如：包含突变的reads在良好reads中的比例
		readsWithMutsPct := float64(stats.TotalReadsWithMuts) / float64(goodAligned) * 100
		writer.WriteString(fmt.Sprintf("ReadsWithMutations_GoodAlignedPct,%.4f%%\n", readsWithMutsPct))

		// 也可以添加其他比例，如各read类型的比例，根据需要
		for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
			count := stats.TotalReadTypeCounts[rt]
			pct := float64(count) / float64(goodAligned) * 100
			writer.WriteString(fmt.Sprintf("%s_GoodAlignedPct,%.4f%%\n", ReadTypeNames[rt], pct))
		}
	} else {
		writer.WriteString("No good aligned reads.\n")
	}

	// 替换个数分布
	writer.WriteString("\n替换个数分布（基于所有比对reads）:\n")
	var counts []int
	for c := range stats.TotalSubstitutionCountDist {
		counts = append(counts, c)
	}
	sort.Ints(counts)
	for _, c := range counts {
		n := stats.TotalSubstitutionCountDist[c]
		pct := float64(n) / float64(stats.TotalAlignedReads) * 100
		writer.WriteString(fmt.Sprintf("Substitutions_%d,%d,%.4f%%\n", c, n, pct))
	}

	// c. 新增：变异事件个数维度
	writer.WriteString("\n变异事件个数维度:\n")

	totalEvents := stats.TotalInsertEventCount + stats.TotalDeleteEventCount + stats.TotalSubstitutionEventCount

	// 插入事件个数
	eventPercentage := 0.0
	if totalEvents > 0 {
		eventPercentage = float64(stats.TotalInsertEventCount) / float64(totalEvents) * 100
	}
	writer.WriteString(fmt.Sprintf("InsertEvents,%d,%.4f%%,-\n",
		stats.TotalInsertEventCount, eventPercentage))

	// 缺失事件个数
	eventPercentage = 0.0
	if totalEvents > 0 {
		eventPercentage = float64(stats.TotalDeleteEventCount) / float64(totalEvents) * 100
	}
	writer.WriteString(fmt.Sprintf("DeleteEvents,%d,%.4f%%,-\n",
		stats.TotalDeleteEventCount, eventPercentage))

	// 替换事件个数
	eventPercentage = 0.0
	if totalEvents > 0 {
		eventPercentage = float64(stats.TotalSubstitutionEventCount) / float64(totalEvents) * 100
	}
	writer.WriteString(fmt.Sprintf("SubstitutionEvents,%d,%.4f%%,-\n",
		stats.TotalSubstitutionEventCount, eventPercentage))

	// d. 新增：碱基个数维度
	writer.WriteString("\n碱基个数维度（变异长度累加）:\n")

	totalBases := stats.TotalInsertBaseTotal + stats.TotalDeleteBaseTotal + stats.TotalSubstitutionBaseTotal

	// 插入碱基总数
	basePercentage := 0.0
	if totalBases > 0 {
		basePercentage = float64(stats.TotalInsertBaseTotal) / float64(totalBases) * 100
	}
	writer.WriteString(fmt.Sprintf("InsertBases,%d,%.4f%%,-\n",
		stats.TotalInsertBaseTotal, basePercentage))

	// 缺失碱基总数
	basePercentage = 0.0
	if totalBases > 0 {
		basePercentage = float64(stats.TotalDeleteBaseTotal) / float64(totalBases) * 100
	}
	writer.WriteString(fmt.Sprintf("DeleteBases,%d,%.4f%%,-\n",
		stats.TotalDeleteBaseTotal, basePercentage))

	// 替换碱基总数
	basePercentage = 0.0
	if totalBases > 0 {
		basePercentage = float64(stats.TotalSubstitutionBaseTotal) / float64(totalBases) * 100
	}
	writer.WriteString(fmt.Sprintf("SubstitutionBases,%d,%.4f%%,-\n",
		stats.TotalSubstitutionBaseTotal, basePercentage))

	// e. 汇总信息
	writer.WriteString("\n汇总信息:\n")
	writer.WriteString(fmt.Sprintf("TotalReads,%d,100.0000%%,100.0000%%\n", totalReads))
	writer.WriteString(fmt.Sprintf("AlignedReads,%d,100.0000%%,%.4f%%\n",
		alignedReads, float64(alignedReads)/float64(totalReads)*100))
	writer.WriteString(fmt.Sprintf("ReadsWithMutations,%d,%.4f%%,%.4f%%\n",
		stats.TotalReadsWithMuts,
		float64(stats.TotalReadsWithMuts)/float64(alignedReads)*100,
		float64(stats.TotalReadsWithMuts)/float64(totalReads)*100))

	// 以追加方式或重新生成整个文件
	// 为了简洁，我们在原有writeReadTypeStats末尾添加新section
	// 假设我们已经写完了原有内容，现在追加
	writer.WriteString("\n细分类统计（事件数）:\n")
	// 缺失
	for st, cnt := range stats.TotalDeleteSubtypeEvents {
		name := "Del1"
		if st == Del2 {
			name = "Del2"
		}
		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%\n", name, cnt,
			float64(cnt)/float64(stats.TotalDeleteEventCount)*100))
	}
	// 插入
	for st, cnt := range stats.TotalInsertSubtypeEvents {
		name := ""
		switch st {
		case Dup1:
			name = "Dup1"
		case Dup2:
			name = "Dup2"
		case DupDup:
			name = "DupDup"
		case Ins1:
			name = "Ins1"
		case Ins2:
			name = "Ins2"
		case Ins3:
			name = "Ins3"
		}
		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%\n", name, cnt,
			float64(cnt)/float64(stats.TotalInsertEventCount)*100))
	}
	// 替换
	for st, cnt := range stats.TotalSubstitutionSubtypeEvents {
		name := ""
		switch st {
		case DupDel:
			name = "DupDel"
		case DelDup:
			name = "DelDup"
		case Mismatch:
			name = "Mismatch"
		}
		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%\n", name, cnt,
			float64(cnt)/float64(stats.TotalSubstitutionEventCount)*100))
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
		totalCount := stats.TotalInsertLengthDist[length]
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
		totalCount := stats.TotalDeleteLengthDist[length]
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

	// 4. 详细类型组合统计
	filename = filepath.Join(outputDir, "detailed_type_combinations.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("DetailedType,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	// 收集所有详细类型
	allDetailedTypes := make(map[string]bool)
	for _, sampleStats := range stats.Samples {
		sampleStats.RLock()

		for typeKey := range sampleStats.DetailedTypeCounts {
			allDetailedTypes[typeKey] = true
		}

		sampleStats.RUnlock()
	}

	// 转换为排序的切片
	var detailedTypes []string
	for typeKey := range allDetailedTypes {
		detailedTypes = append(detailedTypes, typeKey)
	}
	sort.Strings(detailedTypes)

	for _, typeKey := range detailedTypes {
		totalCount := stats.TotalDetailedTypeCounts[typeKey]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", typeKey))

		// 各样本计数
		sampleCounts := []string{}
		// 各样本百分比
		samplePercents := []string{}
		for sampleName, sampleStats := range stats.Samples {
			sampleStats.RLock()

			count := sampleStats.DetailedTypeCounts[typeKey]
			if count > 0 {
				sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				percent := 0.0
				if sampleStats.AlignedReads > 0 {
					percent = float64(count) / float64(sampleStats.AlignedReads) * 100
				}
				samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))
			}

			sampleStats.RUnlock()
		}

		writer.WriteString(strings.Join(sampleCounts, ";") + ",")
		writer.WriteString(fmt.Sprintf("%d,", totalCount))

		writer.WriteString(strings.Join(samplePercents, ";") + ",")
		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("详细类型组合统计已写入: %s\n", filename)

	// 5. 比对率和变异比例（分母为比对reads）
	filename = filepath.Join(outputDir, "alignment_and_mutation_rates.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Sample,TotalReads,AlignedReads,AlignmentRate,ReadsWithMutations,MutationReadRate,TotalMutations,MutationRatePerRead\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads
		readsWithMuts := sampleStats.ReadsWithMutations
		// 计算总突变数
		totalMutations := sampleStats.TotalMutations

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
	for _, count := range stats.TotalMutations {
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

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 总计行
	totalSubstitution := 0
	totalInsertion := 0
	totalDeletion := 0

	for _, sampleName := range sampleNames {
		sampleStats, exists := stats.Samples[sampleName]
		if !exists {
			continue
		}
		sampleStats.RLock()

		totalReads := sampleStats.ReadCounts
		alignedReads := sampleStats.AlignedReads

		// 获取碱基计数
		substitutionBases := sampleStats.MutationBaseCounts["substitution"]
		insertionBases := sampleStats.MutationBaseCounts["insertion"]
		deletionBases := sampleStats.MutationBaseCounts["deletion"]

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

		writer.WriteString(fmt.Sprintf("%s,%d,%d,%d,%d,%d,%.4f,%.4f,%.4f\n",
			sampleName, totalReads, alignedReads,
			substitutionBases, insertionBases, deletionBases,
			substitutionRate, insertionRate, deletionRate))

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

		for _, sampleName := range sampleNames {
			sampleStats, exists := stats.Samples[sampleName]
			if !exists {
				continue
			}
			sampleStats.RLock()

			count := sampleStats.DeleteBaseCounts[base]
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
	for seq := range stats.TotalInsertBaseCounts {
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
		totalCount := stats.TotalInsertBaseCounts[seq]

		// 各样本计数
		sampleCounts := []string{}
		for _, sampleName := range sampleNames {
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
	for posKey, totalCount := range stats.TotalDeletePositionCounts {
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
	for posKey := range stats.TotalDeletePositionCounts {
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

	// 添加样本列
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.Samples {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
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
			count := stats.TotalDeletePositionCounts[posKey]
			row += fmt.Sprintf(",%d", count)
		}

		// 各样本数据
		for _, sampleName := range sampleNames {
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

func mainWrite(outputDir string, stats *MutationStats) {
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

}

func mainPrint(stats *MutationStats) {
	// 打印总体统计
	fmt.Printf("\n处理完成!\n")
	fmt.Printf("总体统计:\n")
	fmt.Printf("  总reads数: %d\n", stats.TotalReadCount)
	fmt.Printf("  总包含突变reads数: %d\n", stats.TotalReadsWithMuts)
	fmt.Printf("  Read类型分布:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		if count > 0 {
			percentage := float64(count) / float64(stats.TotalReadCount) * 100
			fmt.Printf("    %s: %d (%.2f%%)\n", ReadTypeNames[rt], count, percentage)
		}
	}
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
	sampleNames := getSortedSampleNames(stats)

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()

		aligned := sampleStats.AlignedReads
		total := sampleStats.ReadCounts

		// 缺失细分类
		for st, cnt := range sampleStats.DeleteSubtypeReads {
			subtypeName := "Del1"
			if st == Del2 {
				subtypeName = "Del2"
			}
			writer.WriteString(fmt.Sprintf("%s,Deletion,%s,%d,%d,%d,%d,%d\n",
				sampleName, subtypeName, cnt,
				sampleStats.DeleteSubtypeEvents[st],
				sampleStats.DeleteSubtypeBases[st],
				aligned, total))
		}
		// 插入细分类
		insertNames := map[InsertionSubtype]string{
			Dup1: "Dup1", Dup2: "Dup2", DupDup: "DupDup",
			Ins1: "Ins1", Ins2: "Ins2", Ins3: "Ins3",
		}
		for st, cnt := range sampleStats.InsertSubtypeReads {
			writer.WriteString(fmt.Sprintf("%s,Insertion,%s,%d,%d,%d,%d,%d\n",
				sampleName, insertNames[st], cnt,
				sampleStats.InsertSubtypeEvents[st],
				sampleStats.InsertSubtypeBases[st],
				aligned, total))
		}
		// 替换细分类
		substNames := map[SubstitutionSubtype]string{
			DupDel: "DupDel", DelDup: "DelDup", Mismatch: "Mismatch",
		}
		for st, cnt := range sampleStats.SubstitutionSubtypeReads {
			writer.WriteString(fmt.Sprintf("%s,Substitution,%s,%d,%d,%d,%d,%d\n",
				sampleName, substNames[st], cnt,
				sampleStats.SubstitutionSubtypeEvents[st],
				sampleStats.SubstitutionSubtypeBases[st],
				aligned, total))
		}
		sampleStats.RUnlock()
	}

	// 汇总行
	writer.WriteString("Total,,,,,\n")
	// 缺失汇总
	for st, cnt := range stats.TotalDeleteSubtypeReads {
		subtypeName := "Del1"
		if st == Del2 {
			subtypeName = "Del2"
		}
		writer.WriteString(fmt.Sprintf("Total,Deletion,%s,%d,%d,%d,%d,%d\n",
			subtypeName, cnt,
			stats.TotalDeleteSubtypeEvents[st],
			stats.TotalDeleteSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount))
	}
	// 插入汇总
	for st, cnt := range stats.TotalInsertSubtypeReads {
		writer.WriteString(fmt.Sprintf("Total,Insertion,%s,%d,%d,%d,%d,%d\n",
			InsertNames[st], cnt,
			stats.TotalInsertSubtypeEvents[st],
			stats.TotalInsertSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount))
	}
	// 替换汇总
	for st, cnt := range stats.TotalSubstitutionSubtypeReads {
		writer.WriteString(fmt.Sprintf("Total,Substitution,%s,%d,%d,%d,%d,%d\n",
			SubstNames[st], cnt,
			stats.TotalSubstitutionSubtypeEvents[st],
			stats.TotalSubstitutionSubtypeBases[st],
			stats.TotalAlignedReads, stats.TotalReadCount))
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
			writer.WriteString(fmt.Sprintf("%s,%s,%d,%d,%d\n",
				comb, sampleName, cnt, aligned, total))
		}
		sampleStats.RUnlock()
	}

	// 汇总组合统计
	for comb, cnt := range stats.TotalSubtypeCombinationCounts {
		writer.WriteString(fmt.Sprintf("%s,Total,%d,%d,%d\n",
			comb, cnt, stats.TotalAlignedReads, stats.TotalReadCount))
	}

	writer.Flush()
	fmt.Printf("细分类组合统计已写入: %s\n", filename)

	return nil
}

// 辅助函数：获取排序后的样本名列表
func getSortedSampleNames(stats *MutationStats) []string {
	var names []string
	for name := range stats.Samples {
		names = append(names, name)
	}
	if excelFile != "" && len(sampleOrder) > 0 {
		// 按 Excel 顺序排序
		orderMap := make(map[string]int)
		for i, n := range sampleOrder {
			orderMap[n] = i
		}
		sort.Slice(names, func(i, j int) bool {
			// 如果两个都在 sampleOrder 中，按顺序比较；否则按字母序
			oi, ei := orderMap[names[i]]
			oj, ej := orderMap[names[j]]
			if ei && ej {
				return oi < oj
			}
			if ei {
				return true
			}
			if ej {
				return false
			}
			return names[i] < names[j]
		})
	} else {
		sort.Strings(names)
	}
	return names
}

// ========== 新增输出函数：样品-位置详细统计 ==========
func writePositionDetailedStats(stats *MutationStats, outputDir string) error {
	sampleNames := getSortedSampleNames(stats)

	// 汇总数据结构：修正后位置 -> TotalPositionDetail
	totalPosStats := make(map[int]*TotalPositionDetail)

	for _, sampleName := range sampleNames {
		sampleStats := stats.Samples[sampleName]
		sampleStats.RLock()

		refLength := sampleStats.RefLength
		headCut := sampleStats.HeadCut
		tailCut := sampleStats.TailCut
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
		writer.WriteString("pos,depth,match_pure,match_with_ins,mismatch_pure,mismatch_with_ins,insertion,deletion,perfect_reads,perfect_upto_pos\n")

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
				fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
					correctedPos, detail.Depth,
					detail.MatchPure, detail.MatchWithIns,
					detail.MismatchPure, detail.MismatchWithIns,
					detail.Insertion, detail.Deletion,
					detail.PerfectReadsCount, detail.PerfectUptoPosCount,
				),
			)

			// 累加到汇总
			total, ok := totalPosStats[correctedPos]
			if !ok {
				total = &TotalPositionDetail{}
				totalPosStats[correctedPos] = total
			}
			total.Depth += detail.Depth
			total.MatchPure += detail.MatchPure
			total.MatchWithIns += detail.MatchWithIns
			total.MismatchPure += detail.MismatchPure
			total.MismatchWithIns += detail.MismatchWithIns
			total.Insertion += detail.Insertion
			total.Deletion += detail.Deletion
			total.PerfectReadsCount += detail.PerfectReadsCount
			total.PerfectUptoPosCount += detail.PerfectUptoPosCount
			// 累加该样品的 aligned 和 total
			total.AlignedSum += sampleStats.AlignedReads
			total.TotalSum += sampleStats.ReadCounts
		}

		writer.Flush()
		file.Close()
		sampleStats.RUnlock()
		fmt.Printf("  已写入: %s\n", filename)
	}

	// 写入汇总文件
	filename := filepath.Join(outputDir, "total_position_detailed.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建汇总文件失败: %v", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString("pos,depth,match_pure,match_with_ins,mismatch_pure,mismatch_with_ins,insertion,deletion,perfect_reads,perfect_upto_pos,aligned,total\n")

	var totalPositions []int
	for pos := range totalPosStats {
		totalPositions = append(totalPositions, pos)
	}
	sort.Ints(totalPositions)

	for _, pos := range totalPositions {
		detail := totalPosStats[pos]
		writer.WriteString(
			fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
				pos, detail.Depth,
				detail.MatchPure, detail.MatchWithIns,
				detail.MismatchPure, detail.MismatchWithIns,
				detail.Insertion, detail.Deletion,
				detail.PerfectReadsCount, detail.PerfectUptoPosCount,
				detail.AlignedSum, detail.TotalSum,
			),
		)
	}
	writer.Flush()
	fmt.Printf("  已写入: %s\n", filename)

	return nil
}
