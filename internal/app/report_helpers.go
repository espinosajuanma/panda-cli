package app

import "github.com/espinosajuanma/panda-cli/internal/report"

func relationshipDisplayName(value any) string {
	return report.RelationshipDisplayName(value)
}

func relationshipDisplayNameWithCatalog(value any, catalogNames map[string]string) string {
	return report.RelationshipDisplayNameWithCatalog(value, catalogNames)
}

func labelsDisplayWithCatalog(value any, catalogNames map[string]string) string {
	return report.LabelsDisplayWithCatalog(value, catalogNames)
}

func assigneesDisplay(value any) string {
	return report.AssigneesDisplay(value)
}

func displayStatus(status string) string {
	return report.DisplayStatus(status)
}

func tableCell(value any) string {
	return report.TableCell(value)
}

func isEmptyField(value any) bool {
	return report.IsEmptyField(value)
}

func truncateListCell(value string, width int) string {
	return report.TruncateCell(value, width)
}

func wrapTaskViewValue(value string, width int) []string {
	return report.WrapValue(value, width)
}
