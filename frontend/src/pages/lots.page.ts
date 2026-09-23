import {Component, inject} from '@angular/core';
import {NgFor, NgIf} from '@angular/common';
import {FormsModule} from '@angular/forms';
import {lotApi} from '../api/lots';
import {AppStore} from '../stores/app.store';
import {MoistureStageBadgeComponent} from '../components/common/MoistureStageBadge';

@Component({standalone:true,imports:[FormsModule,NgFor,NgIf,MoistureStageBadgeComponent],template:`
<div class="page-head"><div><p class="eyebrow">02 / LOTS</p><h1>工艺批次</h1><p class="description">按受控状态管理树种、厚度与装载信息。</p></div><button class="button primary" (click)="showForm=!showForm">{{showForm?'取消':'＋ 登记批次'}}</button></div>
<form *ngIf="showForm" class="workspace-panel modal-form" (ngSubmit)="create()"><h2>登记工艺批次</h2><label>所属窑炉<select name="kiln" [(ngModel)]="draft.kiln_id" required><option *ngFor="let kiln of store.kilns()" [value]="kiln.id">{{kiln.kiln_code}} · {{kiln.name}}</option></select></label><div class="form-grid"><label>批次编码<input name="code" [(ngModel)]="draft.lot_code" required></label><label>树种<input name="species" [(ngModel)]="draft.species" required></label></div><div class="form-grid"><label>厚度 mm<input name="thickness" type="number" [(ngModel)]="draft.thickness_mm" required></label><label>体积 m³<input name="volume" type="number" [(ngModel)]="draft.volume_m3" required></label></div><div class="form-grid"><label>初始含水率 %<input name="initial" type="number" [(ngModel)]="draft.initial_moisture_pct" required></label><label>目标含水率 %<input name="target" type="number" [(ngModel)]="draft.target_moisture_pct" required></label></div><button class="button primary" type="submit">保存批次</button><p class="form-error">{{error}}</p></form>
<section class="workspace-panel"><div class="panel-head"><div><h2>批次清单</h2><p>状态迁移与版本控制在服务端执行。</p></div><span class="count">{{store.lots().length}} 条</span></div><div class="table-wrap"><table><thead><tr><th>批次</th><th>树种 / 厚度</th><th>含水率</th><th>状态</th><th>变更</th></tr></thead><tbody><tr *ngFor="let lot of store.lots()"><td><strong>{{lot.lot_code}}</strong></td><td>{{lot.species}} / {{lot.thickness_mm}} mm</td><td>{{lot.initial_moisture_pct}}% → {{lot.target_moisture_pct}}%</td><td><kc-moisture-stage-badge [stage]="lot.lot_state"/></td><td><select #state (change)="transition(lot.id, state.value)"><option value="">更新阶段</option><option value="conditioning">调湿</option><option value="drying">干燥中</option><option value="equalizing">均衡</option><option value="completed">完成</option><option value="aborted">中止</option></select></td></tr></tbody></table></div></section>`})
export class LotsPageComponent {
  readonly store=inject(AppStore);
  showForm=false; error=''; draft={lot_code:'LOT-20260822-A',kiln_id:'',species:'白橡',thickness_mm:32,volume_m3:12,initial_moisture_pct:42,target_moisture_pct:10,quality_grade:'待检'};
  async create(){try{this.draft.kiln_id ||= this.store.kilns()[0]?.id || '';await lotApi.create(this.draft);this.showForm=false;this.error='';await this.store.refresh()}catch(error){this.error=error instanceof Error?error.message:'保存失败'}}
  async transition(id:string,state:string){if(!state)return;try{await lotApi.transition(id,state);this.error='';await this.store.refresh()}catch(error){const message=error instanceof Error?error.message:'状态更新失败';this.error=/missing frozen|missing_plan/.test(message)?'最新曲线计划尚未冻结，批次保留在均衡阶段：请先重新计算、审核并冻结计划。':message}}
}
