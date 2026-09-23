import {Component, inject} from '@angular/core';
import {NgFor, NgIf} from '@angular/common';
import {FormsModule} from '@angular/forms';
import {AppStore} from '../stores/app.store';
import {readingApi} from '../api/readings';
import {DryingCurveChartComponent} from '../components/common/DryingCurveChart';

@Component({standalone:true,imports:[FormsModule,NgFor,NgIf,DryingCurveChartComponent],template:`
<div class="page-head"><div><p class="eyebrow">03 / READINGS</p><h1>含水率工作台</h1><p class="description">导入读数、识别异常，观察中心与表层的干燥梯度。</p></div><button class="button primary" (click)="showForm=!showForm">{{showForm?'取消':'＋ 导入读数'}}</button></div>
<form *ngIf="showForm" class="workspace-panel modal-form" (ngSubmit)="importReading()"><h2>导入含水率读数</h2><label>工艺批次<select name="lot" [(ngModel)]="draft.timber_lot_id" required><option *ngFor="let lot of store.lots()" [value]="lot.id">{{lot.lot_code}} · {{lot.species}}</option></select></label><div class="form-grid"><label>采样位置<select name="position" [(ngModel)]="draft.sample_position"><option value="surface">表层</option><option value="core">中心</option></select></label><label>含水率 %<input name="moisture" type="number" step="0.1" [(ngModel)]="draft.moisture_pct" required></label></div><div class="form-grid"><label>干球 °C<input name="dry" type="number" step="0.1" [(ngModel)]="draft.dry_bulb_c" required></label><label>湿球 °C<input name="wet" type="number" step="0.1" [(ngModel)]="draft.wet_bulb_c" required></label></div><button class="button primary" type="submit">导入一条读数</button><p class="form-error">{{error}}</p></form>
<div class="curve-card"><div><p class="eyebrow">MOISTURE PROFILE</p><h2>{{average()}}<small>% 平均含水率</small></h2><p>质量标记会在安全计算前剔除。</p></div><kc-drying-curve [values]="values()"/></div>
<section class="workspace-panel"><div class="panel-head"><div><h2>读数记录</h2><p>记录保留测量时间与质量状态。</p></div><span class="count">{{store.readings().length}} 条</span></div><div class="table-wrap"><table><thead><tr><th>位置</th><th>含水率</th><th>干湿球</th><th>测量时间</th><th>质量</th></tr></thead><tbody><tr *ngFor="let row of store.readings()"><td>{{row.sample_position}}</td><td>{{row.moisture_pct}}%</td><td>{{row.dry_bulb_c}}°C / {{row.wet_bulb_c}}°C</td><td>{{row.measured_at}}</td><td><span class="state warm">{{row.reading_quality}}</span></td></tr></tbody></table></div></section>`})
export class ReadingsPageComponent {
  readonly store=inject(AppStore);
  showForm=false; error=''; draft={timber_lot_id:'',sample_position:'core',moisture_pct:28,dry_bulb_c:56,wet_bulb_c:42};
  values=()=>this.store.readings().map(x=>x.moisture_pct);
  average=()=>{const values=this.values();return values.length?(values.reduce((sum,value)=>sum+value,0)/values.length).toFixed(1):'—'};
  async importReading(){try{this.draft.timber_lot_id ||= this.store.lots()[0]?.id || '';await readingApi.import({timber_lot_id:this.draft.timber_lot_id,readings:[{sample_position:this.draft.sample_position,moisture_pct:this.draft.moisture_pct,dry_bulb_c:this.draft.dry_bulb_c,wet_bulb_c:this.draft.wet_bulb_c}]});this.showForm=false;await this.store.refresh()}catch(error){this.error=error instanceof Error?error.message:'导入失败'}}
}
