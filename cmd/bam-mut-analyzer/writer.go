package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// writePositionStats 写入各bam各位置各碱基变化组合的个数分布统计
func writePositionStats(stats *MutationStats, outputDir string) error {
	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.PositionDetails {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		posDetails, exists := stats.PositionDetails[sampleName]
		if !exists {
			continue
		}

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
		writer.WriteString("Sample,Position,Ref>Alt,Count,TotalReads,ReadsWithMutations\n")
		writer2 := bufio.NewWriter(file2)
		writer2.WriteString("Sample,Position,Count,TotalReads,ReadsWithMutations\n")

		// 获取样本的总reads数和包含突变的reads数
		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		// 收集所有位置
		var positions []string
		for pos := range posDetails {
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
			for mut := range posDetails[pos] {
				mutations = append(mutations, mut)
			}
			sort.Strings(mutations)

			count2 := 0
			for _, mut := range mutations {
				count := posDetails[pos][mut]
				fmt.Fprintf(writer, "%s,%s,%s,%d,%d,%d\n",
					sampleName, pos, mut, count, totalReads, readsWithMuts)
				count2 += count
			}
			fmt.Fprintf(writer2, "%s,%s,%d,%d,%d\n",
				sampleName, pos, count2, totalReads, readsWithMuts)
		}

		writer.Flush()
		fmt.Printf("  已写入: %s\n", filename)
		writer2.Flush()
		fmt.Printf("  已写入: %s\n", filename2)
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
		for sampleName := range stats.SampleMutations {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		mutCounts, exists := stats.SampleMutations[sampleName]
		if !exists {
			continue
		}

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_mutation_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Sample,Ref>Alt,Count,TotalReads,ReadsWithMutations\n")

		// 获取样本的总reads数和包含突变的reads数
		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		// 收集所有突变
		var mutations []string
		for mut := range mutCounts {
			mutations = append(mutations, mut)
		}
		sort.Strings(mutations)

		for _, mut := range mutations {
			count := mutCounts[mut]
			fmt.Fprintf(writer, "%s,%s,%d,%d,%d\n",
				sampleName, mut, count, totalReads, readsWithMuts)
		}

		writer.Flush()
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

// writeSummaryReport 写入汇总报告
func writeSummaryReport(stats *MutationStats, outputDir string) error {
	// 1. 样本汇总
	filename := filepath.Join(outputDir, "summary_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 收集所有突变类型
	allMutations := make(map[string]bool)
	for _, sampleMuts := range stats.SampleMutations {
		for mut := range sampleMuts {
			allMutations[mut] = true
		}
	}

	// 转换为排序的切片
	var mutationTypes []string
	for mut := range allMutations {
		mutationTypes = append(mutationTypes, mut)
	}
	sort.Strings(mutationTypes)

	// 写入表头
	header := "Sample,Total_Reads,Reads_With_Mutations,Total_Mutations"
	for _, mut := range mutationTypes {
		header += "," + mut
	}
	writer.WriteString(header + "\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.SampleMutations {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
		sampleMuts, exists := stats.SampleMutations[sampleName]
		if !exists {
			continue
		}

		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]
		totalMutations := 0
		for _, count := range sampleMuts {
			totalMutations += count
		}

		var row strings.Builder
		fmt.Fprintf(&row, "%s,%d,%d,%d", sampleName, totalReads, readsWithMuts, totalMutations)
		for _, mut := range mutationTypes {
			count := sampleMuts[mut]
			fmt.Fprintf(&row, ",%d", count)
		}
		writer.WriteString(row.String() + "\n")
	}

	writer.Flush()
	fmt.Printf("样本汇总已写入: %s\n", filename)

	// 2. 突变频谱
	filename = filepath.Join(outputDir, "mutation_spectrum.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Mutation,Count,Frequency\n")

	var mutations []string
	for mut := range stats.TotalMutations {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	totalMutations := 0
	for _, count := range stats.TotalMutations {
		totalMutations += count
	}

	for _, mut := range mutations {
		count := stats.TotalMutations[mut]
		frequency := float64(count) / float64(totalMutations)
		fmt.Fprintf(writer, "%s,%d,%.6f\n", mut, count, frequency)
	}

	writer.Flush()
	fmt.Printf("突变频谱已写入: %s\n", filename)

	return nil
}

// writeReadTypeStats 写入read类型统计
func writeReadTypeStats(stats *MutationStats, outputDir string) error {
	// 1. 各样本的read类型统计
	filename := filepath.Join(outputDir, "read_type_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 写入表头
	header := "Sample,TotalReads"
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		header += "," + ReadTypeNames[rt]
	}
	header += ",ReadsWithMutations"
	writer.WriteString(header + "\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.SampleReadTypeCounts {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
		readTypeCounts, exists := stats.SampleReadTypeCounts[sampleName]
		if !exists {
			continue
		}

		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		row := fmt.Sprintf("%s,%d", sampleName, totalReads)

		// 写入每种read类型的计数
		for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
			count := readTypeCounts[rt]
			row += fmt.Sprintf(",%d", count)
		}

		row += fmt.Sprintf(",%d", readsWithMuts)
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

	// 写入表头
	writer.WriteString("ReadType,Count,Percentage,PercentageInTotalReads\n")

	totalReads := stats.TotalReadCount

	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		percentage := float64(count) / float64(totalReads) * 100

		// 计算在总reads中的百分比
		percentageInTotal := percentage

		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%,%.4f%%\n",
			ReadTypeNames[rt], count, percentage, percentageInTotal))
	}

	// 添加包含突变的reads统计
	readsWithMuts := stats.TotalReadsWithMuts
	percentageWithMuts := float64(readsWithMuts) / float64(totalReads) * 100
	writer.WriteString(fmt.Sprintf("包含突变的reads,%d,%.4f%%,%.4f%%\n",
		readsWithMuts, percentageWithMuts, percentageWithMuts))

	writer.Flush()
	fmt.Printf("汇总read类型统计已写入: %s\n", filename)

	// 3. 详细的read类型统计（包含频率和百分比）
	filename = filepath.Join(outputDir, "read_type_detailed.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 写入表头
	writer.WriteString("ReadType,Sample")
	for _, sampleName := range sampleNames {
		writer.WriteString(fmt.Sprintf(",%s_Count,%s_Percentage", sampleName, sampleName))
	}
	writer.WriteString(",Total_Count,Total_Percentage\n")

	// 写入每种read类型的数据
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		row := fmt.Sprintf("%s,", ReadTypeNames[rt])

		// 写入每个样本的计数和百分比
		for _, sampleName := range sampleNames {
			sampleReadTypeCounts, exists := stats.SampleReadTypeCounts[sampleName]
			if !exists {
				row += "0,0.00%,"
				continue
			}

			count := sampleReadTypeCounts[rt]
			sampleTotalReads := stats.SampleReadCounts[sampleName]
			percentage := 0.0
			if sampleTotalReads > 0 {
				percentage = float64(count) / float64(sampleTotalReads) * 100
			}

			row += fmt.Sprintf("%d,%.2f%%,", count, percentage)
		}

		// 写入总计
		totalCount := stats.TotalReadTypeCounts[rt]
		totalPercentage := float64(totalCount) / float64(totalReads) * 100
		row += fmt.Sprintf("%d,%.2f%%", totalCount, totalPercentage)

		writer.WriteString(row + "\n")
	}

	// 添加包含突变的reads行
	row := "包含突变的reads,"
	for _, sampleName := range sampleNames {
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]
		sampleTotalReads := stats.SampleReadCounts[sampleName]
		percentage := 0.0
		if sampleTotalReads > 0 {
			percentage = float64(readsWithMuts) / float64(sampleTotalReads) * 100
		}

		row += fmt.Sprintf("%d,%.2f%%,", readsWithMuts, percentage)
	}

	row += fmt.Sprintf("%d,%.2f%%", stats.TotalReadsWithMuts,
		float64(stats.TotalReadsWithMuts)/float64(totalReads)*100)

	writer.WriteString(row + "\n")

	writer.Flush()
	fmt.Printf("详细read类型统计已写入: %s\n", filename)

	return nil
}
