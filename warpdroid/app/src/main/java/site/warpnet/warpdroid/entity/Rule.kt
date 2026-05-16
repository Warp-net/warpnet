/* Warpnet - Decentralized Social Network */
package site.warpnet.warpdroid.entity

import com.squareup.moshi.JsonClass

/**
 * Server-published rule shown on the report-flow screen.
 *
 * Warpnet has no central admin, so the list is always empty in practice
 * (see [site.warpnet.warpdroid.components.report.ReportViewModel.loadInstanceRules]),
 * but the type stays as a placeholder for the screen's adapter contract.
 */
@JsonClass(generateAdapter = true)
data class Rule(val id: String, val text: String)
