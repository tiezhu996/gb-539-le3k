import {Component, Input} from '@angular/core';
@Component({selector: 'kc-rule-evidence', standalone: true, template: '<div class="rule-list"><div><strong>{{ rule }}</strong><span>{{ explanation }}</span></div></div>'})
export class RuleEvidenceDrawerComponent { @Input() rule = '规则证据'; @Input() explanation = ''; }
