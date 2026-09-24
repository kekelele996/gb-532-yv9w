import { create } from 'zustand'
import { coverageGapApi } from '../api/coverage-gap'
import type { CoverageEvidence, CoverageGap, DetectCoverage } from '../types/coverage-gap'
import type { GapState } from '../types/enums/gap-severity'

export type GapScope = '' | 'unclaimed' | 'mine'

interface GapStore{items:CoverageGap[];selected:CoverageGap|null;evidence:CoverageEvidence|null;loading:boolean;scope:GapScope;setScope:(scope:GapScope)=>Promise<void>;fetch:()=>Promise<void>;select:(item:CoverageGap)=>void;detect:(body:DetectCoverage)=>Promise<void>;claim:(item:CoverageGap)=>Promise<void>;release:(item:CoverageGap,note:string)=>Promise<void>;transition:(item:CoverageGap,target:GapState,note:string)=>Promise<void>}

const upsert=(items:CoverageGap[],updated:CoverageGap)=>items.some(value=>value.id===updated.id)?items.map(value=>value.id===updated.id?updated:value):[updated,...items]

export const useCoverageGapStore=create<GapStore>((set,get)=>({items:[],selected:null,evidence:null,loading:false,scope:'unclaimed',
  setScope:async(scope)=>{set({scope});await get().fetch()},
  fetch:async()=>{set({loading:true});try{const response=await coverageGapApi.list(get().scope);set(state=>({items:response.data,selected:state.selected?response.data.find(item=>item.id===state.selected?.id)??state.selected:response.data[0]??null}))}finally{set({loading:false})}},
  select:(selected)=>set({selected,evidence:null}),
  detect:async(body)=>{const key=`coverage-${body.survey_area_id}-${body.source_run_ids.join('-')}-${body.algorithm_version}-${body.resolution_m}`;const response=await coverageGapApi.detect(body,key);set(state=>({items:upsert(state.items,response.data.gap),selected:response.data.gap,evidence:response.data.evidence}));},
  claim:async(item)=>{const response=await coverageGapApi.claim(item.id,item.version);set(state=>({items:upsert(state.items,response.data),selected:response.data}))},
  release:async(item,note)=>{const response=await coverageGapApi.release(item.id,item.version,note);set(state=>({items:upsert(state.items,response.data),selected:response.data}))},
  transition:async(item,target,note)=>{const response=await coverageGapApi.transition(item.id,target,item.version,note);set(state=>({items:state.items.map(value=>value.id===item.id?response.data:value),selected:response.data}))}
}))
