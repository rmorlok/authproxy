import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{u as Fe,d as v,H as uo,I as po,J as mo,K as go,L as fo,M as ho,N as bo,O as Je,Q as Pe,T as Co,U as xo,C as _t,E as Rt,F as Pt,G as Lt,s as yo,e as vo,f as So,z as jo,g as wo,h as ko,i as Eo,V as Io,W as Ao,X as To,Y as _o,j as B,l as Qe,Z as Ro,n as Po,o as Lo,p as No,_ as Mo,B as Do,$ as Oo,A as Vo,a0 as zo,c as Fo,a1 as $o,P as Ho,y as Bo,a2 as Go,a as Wo,b as Uo,a3 as Zo}from"./connectionPresentation-4pXddxrB.js";import"./client-6EFRD0gB.js";import{C as P}from"./index-DxM3CGmP.js";import{k as qo,o as Re,G as Yo,j as ee,M as en,i as nn,b as tn,a as Ie,D as on,d as rn,e as sn,g as Ko,C as Ae}from"./connections-iuYSbIhR.js";import{m as j,t as Xo}from"./theme-CKuQCuBO.js";import{r as p}from"./index-yIsmwZOr.js";import{L as an,T as Jo,B as Qo,D as er,A as nr,E as Nt,G as L,a as tr}from"./ConnectionFormStep-BPo4zdyM.js";import{L as or,I as rr,C as sr,a as ar}from"./ConnectionCard-B0fAdeRX.js";import{c as $e}from"./createSvgIcon-ByfhJewD.js";import{a as Mt,O as ir,R as cr,f as cn}from"./index-BXrwOJ9g.js";import{c as lr,e as Le,f as dr,g as ln,P as ur,b as ne,B as W}from"./Button-qoqN5xvQ.js";import{g as Dt,u as pr,T as E}from"./Typography-USnMzuFJ.js";import{u as Te,i as He,l as Be,s as z,a as Ot,n as R,g as Ge,o as U,p as K,A as We,B as Ue,K as dn,L as mr}from"./createSimplePaletteValueFilter-CvlNngyw.js";import{A as Y,C as Ze}from"./Container-DwRkeloD.js";import{B as T,C as Ne}from"./Box-CWJIOfo9.js";import{I as gr}from"./IconButton-T-YZe696.js";import{b as fr,L as Vt,A as hr,C as zt,u as br,d as Cr,e as xr}from"./ConnectorDetail-bgwe0aQo.js";import{C as Ft,a as $t}from"./ConnectorCard-BXQFp9YW.js";import{u as yr}from"./Chip-BT1MOgdz.js";import"./index-M3uX8AIl.js";import"./useThemeProps-2qsqrhXJ.js";import"./Close-BpyWLyqp.js";import"./Stack-L6r6t084.js";import"./ConnectorLogo-D_IJLdbp.js";function un(e){return e.substring(2).toLowerCase()}function vr(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function Sr(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:i,touchEvent:u="onTouchEnd"}=e,m=p.useRef(!1),l=p.useRef(null),g=p.useRef(!1),C=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{g.current=!0},0),()=>{g.current=!1}),[]);const a=lr(qo(n),l),f=Le(s=>{const h=C.current;C.current=!1;const k=Re(l.current);if(!g.current||!l.current||"clientX"in s&&vr(s,k))return;if(m.current){m.current=!1;return}let b;s.composedPath?b=s.composedPath().includes(l.current):b=!k.documentElement.contains(s.target)||l.current.contains(s.target),!b&&(o||!h)&&i(s)}),A=s=>h=>{C.current=!0;const k=n.props[s];k&&k(h)},x={ref:a};return u!==!1&&(x[u]=A(u)),p.useEffect(()=>{if(u!==!1){const s=un(u),h=Re(l.current),k=()=>{m.current=!0};return h.addEventListener(s,f),h.addEventListener("touchmove",k),()=>{h.removeEventListener(s,f),h.removeEventListener("touchmove",k)}}},[f,u]),r!==!1&&(x[r]=A(r)),p.useEffect(()=>{if(r!==!1){const s=un(r),h=Re(l.current);return h.addEventListener(s,f),()=>{h.removeEventListener(s,f)}}},[f,r]),p.cloneElement(n,x)}const Me=typeof Dt({})=="function",jr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),wr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Ht=(e,n=!1)=>{var u,m;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([l,g])=>{var a,f;const C=e.getColorSchemeSelector(l);C.startsWith("@")?o[C]={":root":{colorScheme:(a=g.palette)==null?void 0:a.mode}}:o[C.replace(/\s*&/,"")]={colorScheme:(f=g.palette)==null?void 0:f.mode}});let r={html:jr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...wr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const i=(m=(u=e.components)==null?void 0:u.MuiCssBaseline)==null?void 0:m.styleOverrides;return i&&(r=[r,i]),r},Ee="mui-ecs",kr=e=>{const n=Ht(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${Ee})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,i])=>{var m,l;const u=e.getColorSchemeSelector(r);u.startsWith("@")?o[u]={[`:root:not(:has(.${Ee}))`]:{colorScheme:(m=i.palette)==null?void 0:m.mode}}:o[u.replace(/\s*&/,"")]={[`&:not(:has(.${Ee}))`]:{colorScheme:(l=i.palette)==null?void 0:l.mode}}}),n},Er=Dt(Me?({theme:e,enableColorScheme:n})=>Ht(e,n):({theme:e})=>kr(e));function Ir(e){const n=Te({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[Me&&t.jsx(Er,{enableColorScheme:r}),!Me&&!r&&t.jsx("span",{className:Ee,style:{display:"none"}}),o]})}function Ar(e){return He("MuiLinearProgress",e)}Be("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const De=4,Oe=Ue`
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
`,Tr=typeof Oe!="string"?We`
        animation: ${Oe} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,Ve=Ue`
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
`,_r=typeof Ve!="string"?We`
        animation: ${Ve} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,ze=Ue`
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
`,Rr=typeof ze!="string"?We`
        animation: ${ze} 3s infinite linear;
      `:null,Pr=e=>{const{classes:n,variant:o,color:r}=e,i={root:["root",`color${R(r)}`,o],dashed:["dashed",`dashedColor${R(r)}`],bar1:["bar","bar1",`barColor${R(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${R(r)}`,o==="buffer"&&`color${R(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return Ge(i,Ar,n)},qe=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),Lr=z("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${R(o.color)}`],n[o.variant]]}})(U(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:qe(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),Nr=z("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${R(o.color)}`]]}})(U(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(K()).map(([n])=>{const o=qe(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),Rr||{animation:`${ze} 3s infinite linear`}),Mr=z("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${R(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(U(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${De}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${De}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Tr||{animation:`${Oe} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),Dr=z("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${R(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(U(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:qe(e,n),transition:`transform .${De}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:_r||{animation:`${Ve} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),Or=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiLinearProgress"}),{className:i,color:u="primary",value:m,valueBuffer:l,variant:g="indeterminate",...C}=r,a={...r,color:u,variant:g},f=Pr(a),A=yr(),x={},s={bar1:{},bar2:{}};if((g==="determinate"||g==="buffer")&&m!==void 0){x["aria-valuenow"]=Math.round(m),x["aria-valuemin"]=0,x["aria-valuemax"]=100;let h=m-100;A&&(h=-h),s.bar1.transform=`translateX(${h}%)`}if(g==="buffer"&&l!==void 0){let h=(l||0)-100;A&&(h=-h),s.bar2.transform=`translateX(${h}%)`}return t.jsxs(Lr,{className:Ot(f.root,i),ownerState:a,role:"progressbar",...x,ref:o,...C,children:[g==="buffer"?t.jsx(Nr,{className:f.dashed,ownerState:a}):null,t.jsx(Mr,{className:f.bar1,ownerState:a,style:s.bar1}),g==="determinate"?null:t.jsx(Dr,{className:f.bar2,ownerState:a,style:s.bar2})]})});function Vr(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:i,resumeHideDuration:u}=e,m=dr();p.useEffect(()=>{if(!i)return;function b(y){y.defaultPrevented||y.key==="Escape"&&(r==null||r(y,"escapeKeyDown"))}return document.addEventListener("keydown",b),()=>{document.removeEventListener("keydown",b)}},[i,r]);const l=Le((b,y)=>{r==null||r(b,y)}),g=Le(b=>{!r||b==null||m.start(b,()=>{l(null,"timeout")})});p.useEffect(()=>(i&&g(n),m.clear),[i,n,g,m]);const C=b=>{r==null||r(b,"clickaway")},a=m.clear,f=p.useCallback(()=>{n!=null&&g(u??n*.5)},[n,u,g]),A=b=>y=>{const S=b.onBlur;S==null||S(y),f()},x=b=>y=>{const S=b.onFocus;S==null||S(y),a()},s=b=>y=>{const S=b.onMouseEnter;S==null||S(y),a()},h=b=>y=>{const S=b.onMouseLeave;S==null||S(y),f()};return p.useEffect(()=>{if(!o&&i)return window.addEventListener("focus",f),window.addEventListener("blur",a),()=>{window.removeEventListener("focus",f),window.removeEventListener("blur",a)}},[o,i,f,a]),{getRootProps:(b={})=>{const y={...ln(e),...ln(b)};return{role:"presentation",...b,...y,onBlur:A(y),onFocus:x(y),onMouseEnter:s(y),onMouseLeave:h(y)}},onClickAway:C}}function zr(e){return He("MuiSnackbarContent",e)}Be("MuiSnackbarContent",["root","message","action"]);const Fr=e=>{const{classes:n}=e;return Ge({root:["root"],action:["action"],message:["message"]},zr,n)},$r=z(ur,{name:"MuiSnackbarContent",slot:"Root"})(U(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(dn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:dn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),Hr=z("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),Br=z("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),Gr=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiSnackbarContent"}),{action:i,className:u,message:m,role:l="alert",...g}=r,C=r,a=Fr(C);return t.jsxs($r,{role:l,elevation:6,className:Ot(a.root,u),ownerState:C,ref:o,...g,children:[t.jsx(Hr,{className:a.message,ownerState:C,children:m}),i?t.jsx(Br,{className:a.action,ownerState:C,children:i}):null]})});function Wr(e){return He("MuiSnackbar",e)}Be("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const Ur=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${R(o.vertical)}${R(o.horizontal)}`]};return Ge(r,Wr,n)},Zr=z("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${R(o.anchorOrigin.vertical)}${R(o.anchorOrigin.horizontal)}`]]}})(U(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),qr=p.forwardRef(function(n,o){const r=Te({props:n,name:"MuiSnackbar"}),i=pr(),u={enter:i.transitions.duration.enteringScreen,exit:i.transitions.duration.leavingScreen},{action:m,anchorOrigin:{vertical:l,horizontal:g}={vertical:"bottom",horizontal:"left"},autoHideDuration:C=null,children:a,className:f,ClickAwayListenerProps:A,ContentProps:x,disableWindowBlurListener:s=!1,message:h,onBlur:k,onClose:b,onFocus:y,onMouseEnter:S,onMouseLeave:Z,open:F,resumeHideDuration:J,slots:d={},slotProps:c={},TransitionComponent:w,transitionDuration:M=u,TransitionProps:{onEnter:q,onExited:$,...Zt}={},...qt}=r,H={...r,anchorOrigin:{vertical:l,horizontal:g},autoHideDuration:C,disableWindowBlurListener:s,TransitionComponent:w,transitionDuration:M},Yt=Ur(H),{getRootProps:Kt,onClickAway:Xt}=Vr({...H}),[Jt,Ke]=p.useState(!0),Qt=_=>{Ke(!0),$&&$(_)},eo=(_,N)=>{Ke(!1),q&&q(_,N)},Q={slots:{transition:w,...d},slotProps:{content:x,clickAwayListener:A,transition:Zt,...c}},[no,to]=ne("root",{ref:o,className:[Yt.root,f],elementType:Zr,getSlotProps:Kt,externalForwardedProps:{...Q,...qt},ownerState:H}),[oo,{ownerState:ro,...so}]=ne("clickAwayListener",{elementType:Sr,externalForwardedProps:Q,getSlotProps:_=>({onClickAway:(...N)=>{var Xe;const O=N[0];(Xe=_.onClickAway)==null||Xe.call(_,...N),!(O!=null&&O.defaultMuiPrevented)&&Xt(...N)}}),ownerState:H}),[ao,io]=ne("content",{elementType:Gr,shouldForwardComponentProp:!0,externalForwardedProps:Q,additionalProps:{message:h,action:m},ownerState:H}),[co,lo]=ne("transition",{elementType:Yo,externalForwardedProps:Q,getSlotProps:_=>({onEnter:(...N)=>{var O;(O=_.onEnter)==null||O.call(_,...N),eo(...N)},onExited:(...N)=>{var O;(O=_.onExited)==null||O.call(_,...N),Qt(...N)}}),additionalProps:{appear:!0,in:F,timeout:M,direction:l==="top"?"down":"up"},ownerState:H});return!F&&Jt?null:t.jsx(oo,{...so,...d.clickAwayListener&&{ownerState:ro},children:t.jsx(no,{...to,children:t.jsx(co,{...lo,children:a||t.jsx(ao,{...io})})})})}),Yr=$e(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"})),Kr=$e(t.jsx("path",{d:"M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2m6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1zm-2 1H8v-6c0-2.48 1.51-4.5 4-4.5s4 2.02 4 4.5z"})),Xr=$e([t.jsx("path",{d:"M12 5.99 19.53 19H4.47zM12 2 1 21h22z"},"0"),t.jsx("path",{d:"M13 16h-2v2h2zm0-6h-2v5h2z"},"1")]),Jr=e=>e.level===Pe.ERROR?t.jsx(Nt,{color:"error",fontSize:"small"}):e.level===Pe.WARNING?t.jsx(Xr,{color:"warning",fontSize:"small"}):t.jsx(rr,{color:"info",fontSize:"small"}),Qr=e=>{try{const n=new URL(e,window.location.origin);return n.origin!==window.location.origin?null:`${n.pathname}${n.search}${n.hash}`}catch{return null}},Bt=()=>{const e=Fe(),n=Mt(),o=v(uo),[r,i]=p.useState(null),[u,m]=p.useState(null),l=!!r,g=!!u,C=v(po),a=v(mo),f=v(go),A=v(fo),x=v(ho),s=p.useMemo(()=>a.filter(d=>!d.viewed).map(d=>d.id),[a]);p.useEffect(()=>{o&&f==="idle"&&e(bo())},[o,e,f]);const h=d=>{i(d.currentTarget)},k=()=>{i(null)},b=()=>{k(),e(xo())},y=d=>{m(d.currentTarget),s.length>0&&e(Co(s))},S=()=>{m(null)},Z=d=>{if(!d.action_url||!d.can_action)return;S();const c=Qr(d.action_url);if(c){n(c);return}window.location.href=d.action_url},F=C.length==0?"":C.map((d,c)=>t.jsx(qr,{open:!0,autoHideDuration:6e3,onClose:()=>e(Je(c)),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(Y,{onClose:()=>e(Je(c)),severity:d.type,sx:{width:"100%"},children:d.message})},d.id)),J=a.length===0?t.jsx(ee,{disabled:!0,children:t.jsx(an,{primary:"No notifications",primaryTypographyProps:{variant:"body2",color:"text.secondary"}})}):a.map(d=>t.jsxs(ee,{disableRipple:!0,sx:{alignItems:"flex-start",gap:1.5,maxWidth:420,minWidth:{xs:300,sm:380},py:1.5,whiteSpace:"normal"},children:[t.jsx(or,{sx:{minWidth:32,pt:.25},children:Jr(d)}),t.jsx(an,{primary:d.title,secondary:d.message,primaryTypographyProps:{variant:"subtitle2",fontWeight:d.viewed?500:700},secondaryTypographyProps:{variant:"body2",color:"text.secondary",sx:{mt:.5}}}),d.can_action&&d.action_url&&t.jsx(W,{size:"small",variant:"outlined",onClick:c=>{c.stopPropagation(),Z(d)},sx:{flexShrink:0,mt:.25},children:"Open"})]},d.id));return t.jsxs(T,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[o&&t.jsxs(Ze,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",alignItems:"center",gap:1,pt:{xs:1,sm:2}},children:[t.jsx(Jo,{title:"Open notifications",children:t.jsx(gr,{id:"notifications-button",color:"inherit",size:"small",onClick:y,"aria-controls":g?"notifications-menu":void 0,"aria-haspopup":"true","aria-expanded":g?"true":void 0,"aria-label":"Open notifications",sx:{color:"text.secondary"},children:t.jsx(Qo,{badgeContent:x,color:"warning",invisible:x===0,children:t.jsx(Kr,{fontSize:"small"})})})}),t.jsxs(en,{id:"notifications-menu",anchorEl:u,open:g,onClose:S,MenuListProps:{"aria-labelledby":"notifications-button"},PaperProps:{sx:{mt:1,maxHeight:480,borderRadius:j.radius.panel}},children:[t.jsxs(T,{sx:{px:2,py:1.25},children:[t.jsx(E,{variant:"subtitle1",component:"p",sx:{fontWeight:700},children:"Notifications"}),f==="failed"&&t.jsx(E,{variant:"body2",color:"error",children:A??"Could not load notifications"})]}),t.jsx(er,{}),J]}),t.jsx(W,{id:"account-button",onClick:h,color:"inherit",size:"small",endIcon:t.jsx(nr,{alt:o,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":l?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":l?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(E,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:o})}),t.jsxs(en,{id:"account-menu",anchorEl:r,open:l,onClose:k,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(ee,{disabled:!0,children:t.jsx(E,{variant:"body2",children:o})}),t.jsx(ee,{onClick:b,children:"Logout"})]})]}),t.jsxs(T,{component:"main",sx:{flexGrow:1},children:[t.jsx(ir,{}),F]})]})};Bt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Gt=()=>{const e=Fe(),n=Mt(),o=v(_t),r=v(Rt),i=v(Pt),{cancelForm:u,connect:m,currentFormStep:l,formSubmitError:g,isConnecting:C,isSubmittingForm:a,submitForm:f}=fr();p.useEffect(()=>{r==="idle"&&e(Lt())},[r,e]);const A=p.useCallback(s=>{n(`/connectors/${encodeURIComponent(s)}`)},[n]);let x;return r==="loading"?x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx($t,{})},s))}):r==="failed"?x=t.jsx(Y,{severity:"error",children:i}):o.length===0?x=t.jsx(T,{sx:{textAlign:"center",py:j.spacing.pageY},children:t.jsx(E,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):x=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:o.map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ft,{connector:s,onConnect:m,onDetails:A,isConnecting:C})},s.id))}),t.jsxs(Ze,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(E,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:j.spacing.headerGap},children:[C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(Ne,{size:24,sx:{mr:1}}),t.jsx(E,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(W,{component:Vt,to:"/connections",startIcon:t.jsx(hr,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),x,t.jsx(zt,{currentFormStep:l,formSubmitError:g,isSubmittingForm:a,onCancel:u,onSubmit:f})]})};Gt.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const Wt=()=>{const e=Fe(),[n,o]=br(),r=v(yo),i=v(vo),u=v(So),m=v(_t),l=v(Rt),g=v(Pt),C=v(jo),a=v(wo),f=v(ko),A=v(Eo),x=v(Io),s=v(Ao),h=v(To),k=v(_o);p.useEffect(()=>{i==="idle"&&e(B()),l==="idle"&&e(Lt())},[i,l,e]),p.useEffect(()=>{const c=n.get("setup"),w=n.get("connection_id");c==="pending"&&w&&(e(Qe(w)),n.delete("setup"),n.delete("connection_id"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!x)return;const c=window.setInterval(()=>{e(Qe(x))},2e3);return()=>window.clearInterval(c)},[x,e]),p.useEffect(()=>{if(!k)return;e(B());const c=window.setTimeout(()=>{e(Ro())},3500);return()=>window.clearTimeout(c)},[e,k]);const b=p.useCallback((c,w)=>{const M=(a==null?void 0:a.stepId)??"";e(Po({connectionId:c,stepId:M,data:w,returnToUrl:window.location.href})).then(q=>{if(q.meta.requestStatus==="fulfilled"){const $=q.payload;nn($)?window.location.href=$.redirect_url:tn($)?e(B()):e(B())}})},[e,a]),y=p.useCallback(()=>{const c=a==null?void 0:a.connectionId,w=c?r.find(M=>M.id===c):void 0;w&&w.state===Ie.CONFIGURED&&e(Lo(w.id)),e(No())},[e,a,r]),S=p.useCallback(()=>{s&&e(Mo({connectionId:s.connectionId,returnToUrl:window.location.href})).then(c=>{if(c.meta.requestStatus==="fulfilled"){const w=c.payload;w.type==="redirect"&&w.redirect_url&&(window.location.href=w.redirect_url)}})},[e,s]),Z=p.useCallback(()=>{s&&e(Do(s.connectionId)).then(()=>{e(Oo()),e(B())})},[e,s]),F=p.useCallback(c=>{e(Vo({connectorId:c,returnToUrl:`${window.location.origin}/connections`})).then(w=>{if(w.meta.requestStatus==="fulfilled"){const M=w.payload;nn(M)?window.location.href=M.redirect_url:tn(M)&&e(B())}})},[e]),J=()=>l==="loading"||l==="idle"?t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx($t,{})},`connector-skeleton-${c}`))}):l==="failed"?t.jsx(Y,{severity:"error",children:g}):m.length===0?t.jsx(T,{sx:{py:3},children:t.jsx(E,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:m.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ft,{connector:c,onConnect:F,isConnecting:C})},c.id))});let d;return i==="loading"?d=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(ar,{})},`connection-skeleton-${c}`))}):i==="failed"?d=t.jsx(Y,{severity:"error",children:u}):r.length===0?d=t.jsxs(t.Fragment,{children:[t.jsxs(T,{sx:{border:1,borderColor:j.card.borderColor,borderRadius:j.radius.panel,bgcolor:j.card.surface,mb:j.spacing.sectionGap,p:j.spacing.panelPadding},children:[t.jsx(E,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(E,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),C&&t.jsxs(T,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(Ne,{size:24,sx:{mr:1}}),t.jsx(E,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(T,{children:[t.jsx(E,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),J()]})]}):d=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:r.map(c=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(sr,{connection:c,highlightNew:c.id===k})},c.id))}),t.jsxs(Ze,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(E,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(W,{variant:"contained",color:"primary",startIcon:t.jsx(tr,{}),component:Vt,to:"/connectors",children:"Connect More"})]}),d,t.jsx(zt,{currentFormStep:a,formSubmitError:A,isSubmittingForm:f,onCancel:y,onSubmit:b}),t.jsxs(on,{open:x!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(rn,{sx:{pb:1},children:"Verifying connection"}),t.jsx(sn,{dividers:!0,children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:j.spacing.headerGap,py:3},children:[t.jsx(Yr,{color:"primary",sx:{fontSize:40}}),t.jsxs(T,{sx:{textAlign:"center"},children:[t.jsx(E,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(E,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(Or,{sx:{width:"100%"}})]})})]}),t.jsxs(on,{open:s!==null,onClose:Z,maxWidth:"sm",fullWidth:!0,children:[t.jsx(rn,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Nt,{color:"error"}),t.jsx(E,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(sn,{dividers:!0,children:[t.jsxs(Y,{severity:"error",sx:{mb:2},children:[t.jsx(Cr,{children:"Provider check failed"}),(s==null?void 0:s.message)??"Verification failed"]}),t.jsx(E,{variant:"body2",color:"text.secondary",children:s!=null&&s.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(Ko,{children:[t.jsx(W,{onClick:Z,disabled:h,children:"Cancel setup"}),(s==null?void 0:s.canRetry)&&t.jsx(W,{onClick:S,disabled:h,variant:"contained",startIcon:h?t.jsx(Ne,{size:16}):void 0,children:h?"Retrying setup...":"Retry setup"})]})]})]})};Wt.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const G=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(u=>u[0]).join("").slice(0,2).toUpperCase(),i=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(i)}`},D=[{id:"google-drive",namespace:"root",version:1,state:P.ACTIVE,display_name:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:G("Google Drive","#188038"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:P.ACTIVE,display_name:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:G("Greenhouse","#24a47f"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:P.ACTIVE,display_name:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:G("Google Calendar","#1a73e8"),has_configure:!0,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:P.ACTIVE,display_name:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:G("GMail","#d93025"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:P.ACTIVE,display_name:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:G("pipedrive","#017a5e"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:P.ACTIVE,display_name:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:G("Asana","#f06a6a"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"}],V=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:Ie.CONFIGURED,health_state:Ae.HEALTHY,created_at:"2024-04-01T12:00:00Z",updated_at:"2024-04-01T12:00:00Z",...n}),es=[V(D[0]),V(D[2],{health_state:Ae.UNHEALTHY}),V(D[5],{state:Ie.SETUP}),V(D[4],{state:Ie.DISABLED})],Ye={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},I={items:es,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1,recentlyCompletedConnectionId:null},Ut={items:[],status:"succeeded",error:null,markingViewed:!1};function ns({route:e,connectorsState:n={items:D,status:"succeeded",error:null},connectionsState:o=I,notificationsState:r=Ut}){const i=Fo({reducer:$o({auth:Zo,connectors:Uo,connections:Wo,notifications:Go,toasts:Bo}),preloadedState:{auth:{actor_id:"actor_storybook",status:"authenticated"},connectors:n,connections:o,notifications:r,toasts:{items:[]}}});return t.jsx(Ho,{store:i,children:t.jsxs(mr,{theme:Xo,children:[t.jsx(Ir,{}),t.jsx(cr,{children:t.jsx(cn,{element:t.jsx(Bt,{}),children:t.jsx(cn,{path:"*",element:e==="/connectors"?t.jsx(Gt,{}):e==="/connector-detail"?t.jsx(xr,{connectorId:"google-calendar"}):t.jsx(Wt,{})})})})]})})}const Is={title:"Pages/Marketplace",component:ns,parameters:{layout:"fullscreen"}},_e={viewport:{defaultViewport:"marketplaceMobile"}},X={viewport:{defaultViewport:"marketplaceTablet"}},te={args:{route:"/connectors"}},oe={args:{route:"/connector-detail"}},re={args:{route:"/connector-detail"},parameters:_e},se={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},ae={args:{route:"/connections"}},ie={args:{route:"/connections"},parameters:_e},ce={args:{route:"/connections"},parameters:X},le={args:{route:"/connections",connectionsState:{...I,items:[V(D[2],{health_state:Ae.UNHEALTHY})]}}},de={args:{route:"/connections",connectionsState:{...I,items:[V(D[2],{health_state:Ae.UNHEALTHY}),V(D[5],{setup_step_id:"select-workspace"})]},notificationsState:{...Ut,items:[{id:"ntf_reauth",key:"connection:cxn_google-calendar:auth_required",level:Pe.WARNING,state:zo.ACTIVE,resource_type:"connection",resource_id:"cxn_google-calendar",namespace:"root",title:"Connection requires re-authentication",message:"Reconnect this connection to continue using it.",action_url:"/connections/cxn_google-calendar?action=reauth",can_action:!0,viewed:!1,created_at:"2026-07-12T12:00:00Z",updated_at:"2026-07-12T12:00:00Z"}]}}},ue={args:{route:"/connections",connectionsState:{...I,items:[V(D[2]),V(D[0])]}}},pe={args:{route:"/connections",connectionsState:{...I,items:[]}}},me={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:_e},ge={args:{route:"/connections",connectionsState:{...I,items:[]}},parameters:X},fe={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...I,items:[]}}},he={args:{route:"/connectors"},parameters:_e},be={args:{route:"/connectors"},parameters:X},Ce={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye}}},xe={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye}},parameters:X},ye={args:{route:"/connections",connectionsState:{...I,currentFormStep:Ye,submittingForm:!0}}},ve={args:{route:"/connections",connectionsState:{...I,verifyingConnectionId:"cxn_google-calendar"}}},Se={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},je={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:X},we={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},ke={args:{route:"/connections",connectionsState:{...I,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var pn,mn,gn;te.parameters={...te.parameters,docs:{...(pn=te.parameters)==null?void 0:pn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(gn=(mn=te.parameters)==null?void 0:mn.docs)==null?void 0:gn.source}}};var fn,hn,bn;oe.parameters={...oe.parameters,docs:{...(fn=oe.parameters)==null?void 0:fn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(bn=(hn=oe.parameters)==null?void 0:hn.docs)==null?void 0:bn.source}}};var Cn,xn,yn;re.parameters={...re.parameters,docs:{...(Cn=re.parameters)==null?void 0:Cn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(yn=(xn=re.parameters)==null?void 0:xn.docs)==null?void 0:yn.source}}};var vn,Sn,jn;se.parameters={...se.parameters,docs:{...(vn=se.parameters)==null?void 0:vn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(jn=(Sn=se.parameters)==null?void 0:Sn.docs)==null?void 0:jn.source}}};var wn,kn,En;ae.parameters={...ae.parameters,docs:{...(wn=ae.parameters)==null?void 0:wn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(En=(kn=ae.parameters)==null?void 0:kn.docs)==null?void 0:En.source}}};var In,An,Tn;ie.parameters={...ie.parameters,docs:{...(In=ie.parameters)==null?void 0:In.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(Tn=(An=ie.parameters)==null?void 0:An.docs)==null?void 0:Tn.source}}};var _n,Rn,Pn;ce.parameters={...ce.parameters,docs:{...(_n=ce.parameters)==null?void 0:_n.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(Pn=(Rn=ce.parameters)==null?void 0:Rn.docs)==null?void 0:Pn.source}}};var Ln,Nn,Mn;le.parameters={...le.parameters,docs:{...(Ln=le.parameters)==null?void 0:Ln.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        health_state: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Mn=(Nn=le.parameters)==null?void 0:Nn.docs)==null?void 0:Mn.source}}};var Dn,On,Vn;de.parameters={...de.parameters,docs:{...(Dn=de.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        health_state: ConnectionHealthState.UNHEALTHY
      }), connectionFor(connectors[5], {
        setup_step_id: 'select-workspace'
      })]
    },
    notificationsState: {
      ...baseNotificationsState,
      items: [{
        id: 'ntf_reauth',
        key: 'connection:cxn_google-calendar:auth_required',
        level: NotificationLevel.WARNING,
        state: NotificationState.ACTIVE,
        resource_type: 'connection',
        resource_id: 'cxn_google-calendar',
        namespace: 'root',
        title: 'Connection requires re-authentication',
        message: 'Reconnect this connection to continue using it.',
        action_url: '/connections/cxn_google-calendar?action=reauth',
        can_action: true,
        viewed: false,
        created_at: '2026-07-12T12:00:00Z',
        updated_at: '2026-07-12T12:00:00Z'
      }]
    }
  }
}`,...(Vn=(On=de.parameters)==null?void 0:On.docs)==null?void 0:Vn.source}}};var zn,Fn,$n;ue.parameters={...ue.parameters,docs:{...(zn=ue.parameters)==null?void 0:zn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...($n=(Fn=ue.parameters)==null?void 0:Fn.docs)==null?void 0:$n.source}}};var Hn,Bn,Gn;pe.parameters={...pe.parameters,docs:{...(Hn=pe.parameters)==null?void 0:Hn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Gn=(Bn=pe.parameters)==null?void 0:Bn.docs)==null?void 0:Gn.source}}};var Wn,Un,Zn;me.parameters={...me.parameters,docs:{...(Wn=me.parameters)==null?void 0:Wn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(Zn=(Un=me.parameters)==null?void 0:Un.docs)==null?void 0:Zn.source}}};var qn,Yn,Kn;ge.parameters={...ge.parameters,docs:{...(qn=ge.parameters)==null?void 0:qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(Kn=(Yn=ge.parameters)==null?void 0:Yn.docs)==null?void 0:Kn.source}}};var Xn,Jn,Qn;fe.parameters={...fe.parameters,docs:{...(Xn=fe.parameters)==null?void 0:Xn.docs,source:{originalSource:`{
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
}`,...(Qn=(Jn=fe.parameters)==null?void 0:Jn.docs)==null?void 0:Qn.source}}};var et,nt,tt;he.parameters={...he.parameters,docs:{...(et=he.parameters)==null?void 0:et.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(tt=(nt=he.parameters)==null?void 0:nt.docs)==null?void 0:tt.source}}};var ot,rt,st;be.parameters={...be.parameters,docs:{...(ot=be.parameters)==null?void 0:ot.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(st=(rt=be.parameters)==null?void 0:rt.docs)==null?void 0:st.source}}};var at,it,ct;Ce.parameters={...Ce.parameters,docs:{...(at=Ce.parameters)==null?void 0:at.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(ct=(it=Ce.parameters)==null?void 0:it.docs)==null?void 0:ct.source}}};var lt,dt,ut;xe.parameters={...xe.parameters,docs:{...(lt=xe.parameters)==null?void 0:lt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(ut=(dt=xe.parameters)==null?void 0:dt.docs)==null?void 0:ut.source}}};var pt,mt,gt;ye.parameters={...ye.parameters,docs:{...(pt=ye.parameters)==null?void 0:pt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(gt=(mt=ye.parameters)==null?void 0:mt.docs)==null?void 0:gt.source}}};var ft,ht,bt;ve.parameters={...ve.parameters,docs:{...(ft=ve.parameters)==null?void 0:ft.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(bt=(ht=ve.parameters)==null?void 0:ht.docs)==null?void 0:bt.source}}};var Ct,xt,yt;Se.parameters={...Se.parameters,docs:{...(Ct=Se.parameters)==null?void 0:Ct.docs,source:{originalSource:`{
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
}`,...(yt=(xt=Se.parameters)==null?void 0:xt.docs)==null?void 0:yt.source}}};var vt,St,jt;je.parameters={...je.parameters,docs:{...(vt=je.parameters)==null?void 0:vt.docs,source:{originalSource:`{
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
}`,...(jt=(St=je.parameters)==null?void 0:St.docs)==null?void 0:jt.source}}};var wt,kt,Et;we.parameters={...we.parameters,docs:{...(wt=we.parameters)==null?void 0:wt.docs,source:{originalSource:`{
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
}`,...(Et=(kt=we.parameters)==null?void 0:kt.docs)==null?void 0:Et.source}}};var It,At,Tt;ke.parameters={...ke.parameters,docs:{...(It=ke.parameters)==null?void 0:It.docs,source:{originalSource:`{
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
}`,...(Tt=(At=ke.parameters)==null?void 0:At.docs)==null?void 0:Tt.source}}};const As=["AvailableConnectors","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsWithNotifications","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{te as AvailableConnectors,se as AvailableConnectorsLoading,he as AvailableConnectorsMobile,be as AvailableConnectorsTablet,Ce as ConnectionSetupDialog,xe as ConnectionSetupDialogTablet,ye as ConnectionSetupSubmitting,pe as ConnectionsEmpty,fe as ConnectionsEmptyLoadingConnectors,me as ConnectionsEmptyMobile,ge as ConnectionsEmptyTablet,ue as ConnectionsHealthyActions,le as ConnectionsNeedsAttention,ae as ConnectionsPopulated,ie as ConnectionsPopulatedMobile,ce as ConnectionsPopulatedTablet,de as ConnectionsWithNotifications,oe as ConnectorOverview,re as ConnectorOverviewMobile,Se as VerificationFailedDialog,je as VerificationFailedDialogTablet,ke as VerificationFailedNoRetryDialog,we as VerificationRetryingDialog,ve as VerifyingConnectionDialog,As as __namedExportsOrder,Is as default};
