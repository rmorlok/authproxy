import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{u as Me,d as v,H as Qt,I as eo,J as Ze,K as no,C as St,E as jt,F as wt,G as kt,s as to,e as oo,f as ro,z as so,g as ao,h as io,i as co,L as lo,M as uo,N as po,O as mo,j as G,p as Ye,Q as go,l as fo,m as ho,n as bo,T as Co,B as yo,U as xo,A as vo,c as So,V as jo,P as wo,y as ko,a as Eo,b as Ao,W as To}from"./connectionPresentation-CZp-ZCqP.js";import"./client-BIbib-UV.js";import{C as R}from"./index-C2XWl3Gq.js";import{k as Io,o as Ae,G as Po,M as Ro,j as Ke,i as Xe,g as Je,a as je,D as Qe,b as en,d as nn,f as Lo,C as Ve}from"./connections-DbYXNGaQ.js";import{m as x,t as _o}from"./theme-BsVliGDX.js";import{r as p}from"./index-yIsmwZOr.js";import{c as Do,d as Te,e as Mo,f as tn,P as Vo,b as J,B as W}from"./Button-C8SQhaag.js";import{g as Et,u as Fo,T as w}from"./Typography-Bzksp_jr.js";import{u as we,i as Fe,l as Oe,s as O,a as At,n as P,g as $e,o as N,p as Z,A as ze,B as Be,F as on,H as Oo}from"./createSimplePaletteValueFilter-cJOvZn4l.js";import{A as q,C as Ge}from"./Container-DgeRCtOi.js";import{B as T,C as Ie}from"./Box-CZbgkUlr.js";import{A as $o,G as L,a as zo,E as Bo}from"./ConnectionFormStep-BE9w5bse.js";import{O as Go,a as Ho,R as No,f as rn}from"./index-DrIDE-fq.js";import{u as Uo,L as Tt,A as Wo,C as It,b as qo,d as Zo,e as Yo}from"./ConnectorDetail-BCEVMtis.js";import{C as Pt,a as Rt}from"./ConnectorCard-BqIpOHj5.js";import{C as Ko,a as Xo}from"./ConnectionCard-kD0cGKT5.js";import{c as Jo}from"./createSvgIcon-DAoK9w-K.js";import{u as Qo}from"./Chip-D1gbS2_p.js";import"./index-M3uX8AIl.js";import"./Close-Cl80GyBB.js";import"./IconButton-Cam5V3xF.js";import"./useThemeProps-ezkPUc36.js";import"./Stack-BmubcawB.js";import"./ConnectorLogo-CVdltLkv.js";function sn(e){return e.substring(2).toLowerCase()}function er(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function nr(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:i,touchEvent:l="onTouchEnd"}=e,d=p.useRef(!1),c=p.useRef(null),m=p.useRef(!1),b=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{m.current=!0},0),()=>{m.current=!1}),[]);const a=Do(Io(n),c),u=Te(s=>{const g=b.current;b.current=!1;const E=Ae(c.current);if(!m.current||!c.current||"clientX"in s&&er(s,E))return;if(d.current){d.current=!1;return}let h;s.composedPath?h=s.composedPath().includes(c.current):h=!E.documentElement.contains(s.target)||c.current.contains(s.target),!h&&(o||!g)&&i(s)}),I=s=>g=>{b.current=!0;const E=n.props[s];E&&E(g)},C={ref:a};return l!==!1&&(C[l]=I(l)),p.useEffect(()=>{if(l!==!1){const s=sn(l),g=Ae(c.current),E=()=>{d.current=!0};return g.addEventListener(s,u),g.addEventListener("touchmove",E),()=>{g.removeEventListener(s,u),g.removeEventListener("touchmove",E)}}},[u,l]),r!==!1&&(C[r]=I(r)),p.useEffect(()=>{if(r!==!1){const s=sn(r),g=Ae(c.current);return g.addEventListener(s,u),()=>{g.removeEventListener(s,u)}}},[u,r]),p.cloneElement(n,C)}const Pe=typeof Et({})=="function",tr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),or=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Lt=(e,n=!1)=>{var l,d;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([c,m])=>{var a,u;const b=e.getColorSchemeSelector(c);b.startsWith("@")?o[b]={":root":{colorScheme:(a=m.palette)==null?void 0:a.mode}}:o[b.replace(/\s*&/,"")]={colorScheme:(u=m.palette)==null?void 0:u.mode}});let r={html:tr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...or(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const i=(d=(l=e.components)==null?void 0:l.MuiCssBaseline)==null?void 0:d.styleOverrides;return i&&(r=[r,i]),r},Se="mui-ecs",rr=e=>{const n=Lt(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${Se})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,i])=>{var d,c;const l=e.getColorSchemeSelector(r);l.startsWith("@")?o[l]={[`:root:not(:has(.${Se}))`]:{colorScheme:(d=i.palette)==null?void 0:d.mode}}:o[l.replace(/\s*&/,"")]={[`&:not(:has(.${Se}))`]:{colorScheme:(c=i.palette)==null?void 0:c.mode}}}),n},sr=Et(Pe?({theme:e,enableColorScheme:n})=>Lt(e,n):({theme:e})=>rr(e));function ar(e){const n=we({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[Pe&&t.jsx(sr,{enableColorScheme:r}),!Pe&&!r&&t.jsx("span",{className:Se,style:{display:"none"}}),o]})}function ir(e){return Fe("MuiLinearProgress",e)}Oe("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const Re=4,Le=Be`
  0% {
    left: -35%;
    right: 100%;
  }

  60% {
    left: 100%;
    right: -90%;
  }

  100% {
    left: 100%;
    right: -90%;
  }
`,cr=typeof Le!="string"?ze`
        animation: ${Le} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,_e=Be`
  0% {
    left: -200%;
    right: 100%;
  }

  60% {
    left: 107%;
    right: -8%;
  }

  100% {
    left: 107%;
    right: -8%;
  }
`,lr=typeof _e!="string"?ze`
        animation: ${_e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,De=Be`
  0% {
    opacity: 1;
    background-position: 0 -23px;
  }

  60% {
    opacity: 0;
    background-position: 0 -23px;
  }

  100% {
    opacity: 1;
    background-position: -200px -23px;
  }
`,dr=typeof De!="string"?ze`
        animation: ${De} 3s infinite linear;
      `:null,ur=e=>{const{classes:n,variant:o,color:r}=e,i={root:["root",`color${P(r)}`,o],dashed:["dashed",`dashedColor${P(r)}`],bar1:["bar","bar1",`barColor${P(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${P(r)}`,o==="buffer"&&`color${P(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return $e(i,ir,n)},He=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),pr=O("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${P(o.color)}`],n[o.variant]]}})(N(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(Z()).map(([n])=>({props:{color:n},style:{backgroundColor:He(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),mr=O("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${P(o.color)}`]]}})(N(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(Z()).map(([n])=>{const o=He(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),dr||{animation:`${De} 3s infinite linear`}),gr=O("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(N(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(Z()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${Re}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${Re}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:cr||{animation:`${Le} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),fr=O("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(N(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(Z()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(Z()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:He(e,n),transition:`transform .${Re}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:lr||{animation:`${_e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),hr=p.forwardRef(function(n,o){const r=we({props:n,name:"MuiLinearProgress"}),{className:i,color:l="primary",value:d,valueBuffer:c,variant:m="indeterminate",...b}=r,a={...r,color:l,variant:m},u=ur(a),I=Qo(),C={},s={bar1:{},bar2:{}};if((m==="determinate"||m==="buffer")&&d!==void 0){C["aria-valuenow"]=Math.round(d),C["aria-valuemin"]=0,C["aria-valuemax"]=100;let g=d-100;I&&(g=-g),s.bar1.transform=`translateX(${g}%)`}if(m==="buffer"&&c!==void 0){let g=(c||0)-100;I&&(g=-g),s.bar2.transform=`translateX(${g}%)`}return t.jsxs(pr,{className:At(u.root,i),ownerState:a,role:"progressbar",...C,ref:o,...b,children:[m==="buffer"?t.jsx(mr,{className:u.dashed,ownerState:a}):null,t.jsx(gr,{className:u.bar1,ownerState:a,style:s.bar1}),m==="determinate"?null:t.jsx(fr,{className:u.bar2,ownerState:a,style:s.bar2})]})});function br(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:i,resumeHideDuration:l}=e,d=Mo();p.useEffect(()=>{if(!i)return;function h(y){y.defaultPrevented||y.key==="Escape"&&(r==null||r(y,"escapeKeyDown"))}return document.addEventListener("keydown",h),()=>{document.removeEventListener("keydown",h)}},[i,r]);const c=Te((h,y)=>{r==null||r(h,y)}),m=Te(h=>{!r||h==null||d.start(h,()=>{c(null,"timeout")})});p.useEffect(()=>(i&&m(n),d.clear),[i,n,m,d]);const b=h=>{r==null||r(h,"clickaway")},a=d.clear,u=p.useCallback(()=>{n!=null&&m(l??n*.5)},[n,l,m]),I=h=>y=>{const S=h.onBlur;S==null||S(y),u()},C=h=>y=>{const S=h.onFocus;S==null||S(y),a()},s=h=>y=>{const S=h.onMouseEnter;S==null||S(y),a()},g=h=>y=>{const S=h.onMouseLeave;S==null||S(y),u()};return p.useEffect(()=>{if(!o&&i)return window.addEventListener("focus",u),window.addEventListener("blur",a),()=>{window.removeEventListener("focus",u),window.removeEventListener("blur",a)}},[o,i,u,a]),{getRootProps:(h={})=>{const y={...tn(e),...tn(h)};return{role:"presentation",...h,...y,onBlur:I(y),onFocus:C(y),onMouseEnter:s(y),onMouseLeave:g(y)}},onClickAway:b}}function Cr(e){return Fe("MuiSnackbarContent",e)}Oe("MuiSnackbarContent",["root","message","action"]);const yr=e=>{const{classes:n}=e;return $e({root:["root"],action:["action"],message:["message"]},Cr,n)},xr=O(Vo,{name:"MuiSnackbarContent",slot:"Root"})(N(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(on(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:on(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),vr=O("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),Sr=O("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),jr=p.forwardRef(function(n,o){const r=we({props:n,name:"MuiSnackbarContent"}),{action:i,className:l,message:d,role:c="alert",...m}=r,b=r,a=yr(b);return t.jsxs(xr,{role:c,elevation:6,className:At(a.root,l),ownerState:b,ref:o,...m,children:[t.jsx(vr,{className:a.message,ownerState:b,children:d}),i?t.jsx(Sr,{className:a.action,ownerState:b,children:i}):null]})});function wr(e){return Fe("MuiSnackbar",e)}Oe("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const kr=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${P(o.vertical)}${P(o.horizontal)}`]};return $e(r,wr,n)},Er=O("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${P(o.anchorOrigin.vertical)}${P(o.anchorOrigin.horizontal)}`]]}})(N(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),Ar=p.forwardRef(function(n,o){const r=we({props:n,name:"MuiSnackbar"}),i=Fo(),l={enter:i.transitions.duration.enteringScreen,exit:i.transitions.duration.leavingScreen},{action:d,anchorOrigin:{vertical:c,horizontal:m}={vertical:"bottom",horizontal:"left"},autoHideDuration:b=null,children:a,className:u,ClickAwayListenerProps:I,ContentProps:C,disableWindowBlurListener:s=!1,message:g,onBlur:E,onClose:h,onFocus:y,onMouseEnter:S,onMouseLeave:Ee,open:K,resumeHideDuration:Ue,slots:V={},slotProps:f={},TransitionComponent:j,transitionDuration:D=l,TransitionProps:{onEnter:U,onExited:z,...Vt}={},...Ft}=r,B={...r,anchorOrigin:{vertical:c,horizontal:m},autoHideDuration:b,disableWindowBlurListener:s,TransitionComponent:j,transitionDuration:D},Ot=kr(B),{getRootProps:$t,onClickAway:zt}=br({...B}),[Bt,We]=p.useState(!0),Gt=A=>{We(!0),z&&z(A)},Ht=(A,_)=>{We(!1),U&&U(A,_)},X={slots:{transition:j,...V},slotProps:{content:C,clickAwayListener:I,transition:Vt,...f}},[Nt,Ut]=J("root",{ref:o,className:[Ot.root,u],elementType:Er,getSlotProps:$t,externalForwardedProps:{...X,...Ft},ownerState:B}),[Wt,{ownerState:qt,...Zt}]=J("clickAwayListener",{elementType:nr,externalForwardedProps:X,getSlotProps:A=>({onClickAway:(..._)=>{var qe;const M=_[0];(qe=A.onClickAway)==null||qe.call(A,..._),!(M!=null&&M.defaultMuiPrevented)&&zt(..._)}}),ownerState:B}),[Yt,Kt]=J("content",{elementType:jr,shouldForwardComponentProp:!0,externalForwardedProps:X,additionalProps:{message:g,action:d},ownerState:B}),[Xt,Jt]=J("transition",{elementType:Po,externalForwardedProps:X,getSlotProps:A=>({onEnter:(..._)=>{var M;(M=A.onEnter)==null||M.call(A,..._),Ht(..._)},onExited:(..._)=>{var M;(M=A.onExited)==null||M.call(A,..._),Gt(..._)}}),additionalProps:{appear:!0,in:K,timeout:D,direction:c==="top"?"down":"up"},ownerState:B});return!K&&Bt?null:t.jsx(Wt,{...Zt,...V.clickAwayListener&&{ownerState:qt},children:t.jsx(Nt,{...Ut,children:t.jsx(Xt,{...Jt,children:a||t.jsx(Yt,{...Kt})})})})}),Tr=Jo(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"})),_t=()=>{const e=Me(),n=v(Qt),[o,r]=p.useState(null),i=!!o,l=v(eo),d=a=>{r(a.currentTarget)},c=()=>{r(null)},m=()=>{c(),e(no())},b=l.length==0?"":l.map((a,u)=>t.jsx(Ar,{open:!0,autoHideDuration:6e3,onClose:()=>Ze(u),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(q,{onClose:()=>Ze(u),severity:a.type,sx:{width:"100%"},children:a.message})},a.id));return t.jsxs(T,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[n&&t.jsxs(Ge,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",pt:{xs:1,sm:2}},children:[t.jsx(W,{id:"account-button",onClick:d,color:"inherit",size:"small",endIcon:t.jsx($o,{alt:n,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":i?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":i?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(w,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:n})}),t.jsxs(Ro,{id:"account-menu",anchorEl:o,open:i,onClose:c,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(Ke,{disabled:!0,children:t.jsx(w,{variant:"body2",children:n})}),t.jsx(Ke,{onClick:m,children:"Logout"})]})]}),t.jsxs(T,{component:"main",sx:{flexGrow:1},children:[t.jsx(Go,{}),b]})]})};_t.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Dt=()=>{const e=Me(),n=Ho(),o=v(St),r=v(jt),i=v(wt),{cancelForm:l,connect:d,currentFormStep:c,formSubmitError:m,isConnecting:b,isSubmittingForm:a,submitForm:u}=Uo();p.useEffect(()=>{r==="idle"&&e(kt())},[r,e]);const I=p.useCallback(s=>{n(`/connectors/${encodeURIComponent(s)}`)},[n]);let C;return r==="loading"?C=t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:[1,2,3,4].map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Rt,{})},s))}):r==="failed"?C=t.jsx(q,{severity:"error",children:i}):o.length===0?C=t.jsx(T,{sx:{textAlign:"center",py:x.spacing.pageY},children:t.jsx(w,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):C=t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:o.map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Pt,{connector:s,onConnect:d,onDetails:I,isConnecting:b})},s.id))}),t.jsxs(Ge,{sx:{py:x.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:x.spacing.headerGap,mb:x.spacing.sectionGap},children:[t.jsx(w,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:x.spacing.headerGap},children:[b&&t.jsxs(T,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(Ie,{size:24,sx:{mr:1}}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(W,{component:Tt,to:"/connections",startIcon:t.jsx(Wo,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),C,t.jsx(It,{currentFormStep:c,formSubmitError:m,isSubmittingForm:a,onCancel:l,onSubmit:u})]})};Dt.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const Mt=()=>{const e=Me(),[n,o]=qo(),r=v(to),i=v(oo),l=v(ro),d=v(St),c=v(jt),m=v(wt),b=v(so),a=v(ao),u=v(io),I=v(co),C=v(lo),s=v(uo),g=v(po),E=v(mo);p.useEffect(()=>{i==="idle"&&e(G()),c==="idle"&&e(kt())},[i,c,e]),p.useEffect(()=>{const f=n.get("setup"),j=n.get("connection_id");f==="pending"&&j&&(e(Ye(j)),n.delete("setup"),n.delete("connection_id"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!C)return;const f=window.setInterval(()=>{e(Ye(C))},2e3);return()=>window.clearInterval(f)},[C,e]),p.useEffect(()=>{if(!E)return;e(G());const f=window.setTimeout(()=>{e(go())},3500);return()=>window.clearTimeout(f)},[e,E]);const h=p.useCallback((f,j)=>{const D=(a==null?void 0:a.stepId)??"";e(fo({connectionId:f,stepId:D,data:j,returnToUrl:window.location.href})).then(U=>{if(U.meta.requestStatus==="fulfilled"){const z=U.payload;Xe(z)?window.location.href=z.redirect_url:Je(z)?e(G()):e(G())}})},[e,a]),y=p.useCallback(()=>{const f=a==null?void 0:a.connectionId,j=f?r.find(D=>D.id===f):void 0;j&&j.state===je.CONFIGURED&&e(ho(j.id)),e(bo())},[e,a,r]),S=p.useCallback(()=>{s&&e(Co({connectionId:s.connectionId,returnToUrl:window.location.href})).then(f=>{if(f.meta.requestStatus==="fulfilled"){const j=f.payload;j.type==="redirect"&&j.redirect_url&&(window.location.href=j.redirect_url)}})},[e,s]),Ee=p.useCallback(()=>{s&&e(yo(s.connectionId)).then(()=>{e(xo()),e(G())})},[e,s]),K=p.useCallback(f=>{e(vo({connectorId:f,returnToUrl:`${window.location.origin}/connections`})).then(j=>{if(j.meta.requestStatus==="fulfilled"){const D=j.payload;Xe(D)?window.location.href=D.redirect_url:Je(D)&&e(G())}})},[e]),Ue=()=>c==="loading"||c==="idle"?t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:[1,2,3,4].map(f=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Rt,{})},`connector-skeleton-${f}`))}):c==="failed"?t.jsx(q,{severity:"error",children:m}):d.length===0?t.jsx(T,{sx:{py:3},children:t.jsx(w,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:d.map(f=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Pt,{connector:f,onConnect:K,isConnecting:b})},f.id))});let V;return i==="loading"?V=t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:[1,2,3,4].map(f=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Xo,{})},`connection-skeleton-${f}`))}):i==="failed"?V=t.jsx(q,{severity:"error",children:l}):r.length===0?V=t.jsxs(t.Fragment,{children:[t.jsxs(T,{sx:{border:1,borderColor:x.card.borderColor,borderRadius:x.radius.panel,bgcolor:x.card.surface,mb:x.spacing.sectionGap,p:x.spacing.panelPadding},children:[t.jsx(w,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(w,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),b&&t.jsxs(T,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(Ie,{size:24,sx:{mr:1}}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(T,{children:[t.jsx(w,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),Ue()]})]}):V=t.jsx(L,{container:!0,spacing:x.spacing.gridGap,children:r.map(f=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ko,{connection:f,highlightNew:f.id===E})},f.id))}),t.jsxs(Ge,{sx:{py:x.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:x.spacing.headerGap,mb:x.spacing.sectionGap},children:[t.jsx(w,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(W,{variant:"contained",color:"primary",startIcon:t.jsx(zo,{}),component:Tt,to:"/connectors",children:"Connect More"})]}),V,t.jsx(It,{currentFormStep:a,formSubmitError:I,isSubmittingForm:u,onCancel:y,onSubmit:h}),t.jsxs(Qe,{open:C!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(en,{sx:{pb:1},children:"Verifying connection"}),t.jsx(nn,{dividers:!0,children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:x.spacing.headerGap,py:3},children:[t.jsx(Tr,{color:"primary",sx:{fontSize:40}}),t.jsxs(T,{sx:{textAlign:"center"},children:[t.jsx(w,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(hr,{sx:{width:"100%"}})]})})]}),t.jsxs(Qe,{open:s!==null,onClose:Ee,maxWidth:"sm",fullWidth:!0,children:[t.jsx(en,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Bo,{color:"error"}),t.jsx(w,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(nn,{dividers:!0,children:[t.jsxs(q,{severity:"error",sx:{mb:2},children:[t.jsx(Zo,{children:"Provider check failed"}),(s==null?void 0:s.message)??"Verification failed"]}),t.jsx(w,{variant:"body2",color:"text.secondary",children:s!=null&&s.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(Lo,{children:[t.jsx(W,{onClick:Ee,disabled:g,children:"Cancel setup"}),(s==null?void 0:s.canRetry)&&t.jsx(W,{onClick:S,disabled:g,variant:"contained",startIcon:g?t.jsx(Ie,{size:16}):void 0,children:g?"Retrying setup...":"Retry setup"})]})]})]})};Mt.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const H=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(l=>l[0]).join("").slice(0,2).toUpperCase(),i=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(i)}`},F=[{id:"google-drive",namespace:"root",version:1,state:R.ACTIVE,display_name:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:H("Google Drive","#188038"),has_configure:!1,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:R.ACTIVE,display_name:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:H("Greenhouse","#24a47f"),has_configure:!1,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:R.ACTIVE,display_name:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:H("Google Calendar","#1a73e8"),has_configure:!0,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:R.ACTIVE,display_name:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:H("GMail","#d93025"),has_configure:!1,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:R.ACTIVE,display_name:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:H("pipedrive","#017a5e"),has_configure:!1,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:R.ACTIVE,display_name:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:H("Asana","#f06a6a"),has_configure:!1,versions:1,states:[R.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"}],$=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:je.CONFIGURED,health_state:Ve.HEALTHY,created_at:"2024-04-01T12:00:00Z",updated_at:"2024-04-01T12:00:00Z",...n}),Ir=[$(F[0]),$(F[2],{health_state:Ve.UNHEALTHY}),$(F[5],{state:je.SETUP}),$(F[4],{state:je.DISABLED})],Ne={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},k={items:Ir,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1};function Pr({route:e,connectorsState:n={items:F,status:"succeeded",error:null},connectionsState:o=k}){const r=So({reducer:jo({auth:To,connectors:Ao,connections:Eo,toasts:ko}),preloadedState:{auth:{actor_id:"actor_storybook",status:"authenticated"},connectors:n,connections:o,toasts:{items:[]}}});return t.jsx(wo,{store:r,children:t.jsxs(Oo,{theme:_o,children:[t.jsx(ar,{}),t.jsx(No,{children:t.jsx(rn,{element:t.jsx(_t,{}),children:t.jsx(rn,{path:"*",element:e==="/connectors"?t.jsx(Dt,{}):e==="/connector-detail"?t.jsx(Yo,{connectorId:"google-calendar"}):t.jsx(Mt,{})})})})]})})}const ts={title:"Pages/Marketplace",component:Pr,parameters:{layout:"fullscreen"}},ke={viewport:{defaultViewport:"marketplaceMobile"}},Y={viewport:{defaultViewport:"marketplaceTablet"}},Q={args:{route:"/connectors"}},ee={args:{route:"/connector-detail"}},ne={args:{route:"/connector-detail"},parameters:ke},te={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},oe={args:{route:"/connections"}},re={args:{route:"/connections"},parameters:ke},se={args:{route:"/connections"},parameters:Y},ae={args:{route:"/connections",connectionsState:{...k,items:[$(F[2],{health_state:Ve.UNHEALTHY})]}}},ie={args:{route:"/connections",connectionsState:{...k,items:[$(F[2]),$(F[0])]}}},ce={args:{route:"/connections",connectionsState:{...k,items:[]}}},le={args:{route:"/connections",connectionsState:{...k,items:[]}},parameters:ke},de={args:{route:"/connections",connectionsState:{...k,items:[]}},parameters:Y},ue={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...k,items:[]}}},pe={args:{route:"/connectors"},parameters:ke},me={args:{route:"/connectors"},parameters:Y},ge={args:{route:"/connections",connectionsState:{...k,currentFormStep:Ne}}},fe={args:{route:"/connections",connectionsState:{...k,currentFormStep:Ne}},parameters:Y},he={args:{route:"/connections",connectionsState:{...k,currentFormStep:Ne,submittingForm:!0}}},be={args:{route:"/connections",connectionsState:{...k,verifyingConnectionId:"cxn_google-calendar"}}},Ce={args:{route:"/connections",connectionsState:{...k,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},ye={args:{route:"/connections",connectionsState:{...k,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:Y},xe={args:{route:"/connections",connectionsState:{...k,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},ve={args:{route:"/connections",connectionsState:{...k,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var an,cn,ln;Q.parameters={...Q.parameters,docs:{...(an=Q.parameters)==null?void 0:an.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(ln=(cn=Q.parameters)==null?void 0:cn.docs)==null?void 0:ln.source}}};var dn,un,pn;ee.parameters={...ee.parameters,docs:{...(dn=ee.parameters)==null?void 0:dn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(pn=(un=ee.parameters)==null?void 0:un.docs)==null?void 0:pn.source}}};var mn,gn,fn;ne.parameters={...ne.parameters,docs:{...(mn=ne.parameters)==null?void 0:mn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(fn=(gn=ne.parameters)==null?void 0:gn.docs)==null?void 0:fn.source}}};var hn,bn,Cn;te.parameters={...te.parameters,docs:{...(hn=te.parameters)==null?void 0:hn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(Cn=(bn=te.parameters)==null?void 0:bn.docs)==null?void 0:Cn.source}}};var yn,xn,vn;oe.parameters={...oe.parameters,docs:{...(yn=oe.parameters)==null?void 0:yn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(vn=(xn=oe.parameters)==null?void 0:xn.docs)==null?void 0:vn.source}}};var Sn,jn,wn;re.parameters={...re.parameters,docs:{...(Sn=re.parameters)==null?void 0:Sn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(wn=(jn=re.parameters)==null?void 0:jn.docs)==null?void 0:wn.source}}};var kn,En,An;se.parameters={...se.parameters,docs:{...(kn=se.parameters)==null?void 0:kn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(An=(En=se.parameters)==null?void 0:En.docs)==null?void 0:An.source}}};var Tn,In,Pn;ae.parameters={...ae.parameters,docs:{...(Tn=ae.parameters)==null?void 0:Tn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        health_state: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Pn=(In=ae.parameters)==null?void 0:In.docs)==null?void 0:Pn.source}}};var Rn,Ln,_n;ie.parameters={...ie.parameters,docs:{...(Rn=ie.parameters)==null?void 0:Rn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(_n=(Ln=ie.parameters)==null?void 0:Ln.docs)==null?void 0:_n.source}}};var Dn,Mn,Vn;ce.parameters={...ce.parameters,docs:{...(Dn=ce.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Vn=(Mn=ce.parameters)==null?void 0:Mn.docs)==null?void 0:Vn.source}}};var Fn,On,$n;le.parameters={...le.parameters,docs:{...(Fn=le.parameters)==null?void 0:Fn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...($n=(On=le.parameters)==null?void 0:On.docs)==null?void 0:$n.source}}};var zn,Bn,Gn;de.parameters={...de.parameters,docs:{...(zn=de.parameters)==null?void 0:zn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(Gn=(Bn=de.parameters)==null?void 0:Bn.docs)==null?void 0:Gn.source}}};var Hn,Nn,Un;ue.parameters={...ue.parameters,docs:{...(Hn=ue.parameters)==null?void 0:Hn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    },
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Un=(Nn=ue.parameters)==null?void 0:Nn.docs)==null?void 0:Un.source}}};var Wn,qn,Zn;pe.parameters={...pe.parameters,docs:{...(Wn=pe.parameters)==null?void 0:Wn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(Zn=(qn=pe.parameters)==null?void 0:qn.docs)==null?void 0:Zn.source}}};var Yn,Kn,Xn;me.parameters={...me.parameters,docs:{...(Yn=me.parameters)==null?void 0:Yn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(Xn=(Kn=me.parameters)==null?void 0:Kn.docs)==null?void 0:Xn.source}}};var Jn,Qn,et;ge.parameters={...ge.parameters,docs:{...(Jn=ge.parameters)==null?void 0:Jn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(et=(Qn=ge.parameters)==null?void 0:Qn.docs)==null?void 0:et.source}}};var nt,tt,ot;fe.parameters={...fe.parameters,docs:{...(nt=fe.parameters)==null?void 0:nt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(ot=(tt=fe.parameters)==null?void 0:tt.docs)==null?void 0:ot.source}}};var rt,st,at;he.parameters={...he.parameters,docs:{...(rt=he.parameters)==null?void 0:rt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(at=(st=he.parameters)==null?void 0:st.docs)==null?void 0:at.source}}};var it,ct,lt;be.parameters={...be.parameters,docs:{...(it=be.parameters)==null?void 0:it.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(lt=(ct=be.parameters)==null?void 0:ct.docs)==null?void 0:lt.source}}};var dt,ut,pt;Ce.parameters={...Ce.parameters,docs:{...(dt=Ce.parameters)==null?void 0:dt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  }
}`,...(pt=(ut=Ce.parameters)==null?void 0:ut.docs)==null?void 0:pt.source}}};var mt,gt,ft;ye.parameters={...ye.parameters,docs:{...(mt=ye.parameters)==null?void 0:mt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  },
  parameters: tabletViewport
}`,...(ft=(gt=ye.parameters)==null?void 0:gt.docs)==null?void 0:ft.source}}};var ht,bt,Ct;xe.parameters={...xe.parameters,docs:{...(ht=xe.parameters)==null?void 0:ht.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      },
      retryingConnection: true
    }
  }
}`,...(Ct=(bt=xe.parameters)==null?void 0:bt.docs)==null?void 0:Ct.source}}};var yt,xt,vt;ve.parameters={...ve.parameters,docs:{...(yt=ve.parameters)==null?void 0:yt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'The provider rejected this setup and it cannot be retried.',
        canRetry: false
      }
    }
  }
}`,...(vt=(xt=ve.parameters)==null?void 0:xt.docs)==null?void 0:vt.source}}};const os=["AvailableConnectors","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{Q as AvailableConnectors,te as AvailableConnectorsLoading,pe as AvailableConnectorsMobile,me as AvailableConnectorsTablet,ge as ConnectionSetupDialog,fe as ConnectionSetupDialogTablet,he as ConnectionSetupSubmitting,ce as ConnectionsEmpty,ue as ConnectionsEmptyLoadingConnectors,le as ConnectionsEmptyMobile,de as ConnectionsEmptyTablet,ie as ConnectionsHealthyActions,ae as ConnectionsNeedsAttention,oe as ConnectionsPopulated,re as ConnectionsPopulatedMobile,se as ConnectionsPopulatedTablet,ee as ConnectorOverview,ne as ConnectorOverviewMobile,Ce as VerificationFailedDialog,ye as VerificationFailedDialogTablet,ve as VerificationFailedNoRetryDialog,xe as VerificationRetryingDialog,be as VerifyingConnectionDialog,os as __namedExportsOrder,ts as default};
