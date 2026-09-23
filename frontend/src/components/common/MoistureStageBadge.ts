import {Component, Input} from '@angular/core';
@Component({selector: 'kc-moisture-stage-badge', standalone: true, template: '<span class="state warm">{{ label }}</span>'})
export class MoistureStageBadgeComponent {
  @Input() stage = '';
  get label(): string { return ({queued:'待处理',conditioning:'调湿',drying:'干燥中',equalizing:'均衡',completed:'已完成',aborted:'已中止'} as Record<string,string>)[this.stage] || this.stage; }
}
