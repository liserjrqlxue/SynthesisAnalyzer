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
	for sampleName, posDetails := range stats.PositionDetails {
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
	for sampleName, mutCounts := range stats.SampleMutations {
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
	var header strings.Builder
	header.WriteString("Sample,Total_Reads,Reads_With_Mutations,Total_Mutations")
	for _, mut := range mutationTypes {
		header.WriteString("," + mut)
	}
	writer.WriteString(header.String() + "\n")

	// 收集样本名并排序
	var sampleNames []string
	for sampleName := range stats.SampleMutations {
		sampleNames = append(sampleNames, sampleName)
	}
	sort.Strings(sampleNames)

	// 写入每行数据
	for _, sampleName := range sampleNames {
		sampleMuts := stats.SampleMutations[sampleName]
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
