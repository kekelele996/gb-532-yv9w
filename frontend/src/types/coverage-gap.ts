import type { GeoJSONFeature } from './api'
import type { GapSeverity, GapState } from './enums/gap-severity'
import type { SurveyArea } from './survey-area'

export interface GapClaimant { id:number; username:string; display_name:string }
export interface CoverageGap {
  id:number; survey_area_id:number; source_run_ids:number[]; gap_geojson:GeoJSONFeature; area_square_m:number; gap_ratio:number; severity:GapSeverity
  recommended_line_geojson:GeoJSONFeature; algorithm_version:string; input_hash:string; gap_state:GapState; explanation:string; coverage_ratio:number
  overlap_ratio:number; processing_millis:number; version:number; detected_at:string; deadline_at:string; claimed_by_id:number|null; claimed_at:string|null
  claim_note:string; updated_at:string; survey_area?:SurveyArea; claimed_by?:GapClaimant
}
export interface CoverageEvidence { input_hash:string; coordinate_system:string; algorithm_version:string; source_run_count:number; coverage_ratio:number; overlap_ratio:number; gap_ratio:number; filtered_fragments:number; processing_millis:number; decision_boundary_note:string }
export interface CoverageResult { gap:CoverageGap; evidence:CoverageEvidence; idempotent:boolean }
export interface DetectCoverage { survey_area_id:number; source_run_ids:number[]; algorithm_version:string; resolution_m:number }
export type GapClaimFilter = 'all' | 'unclaimed' | 'claimed'
