"use strict";

var TaskGraph = (function() {

function svgElement(tag, attr, children) {
	var e = document.createElementNS("http://www.w3.org/2000/svg",tag);
	children && e.append(...[children].flat(999));
	if (typeof(attr) == "function") {
		attr(e);
	} else if (typeof(attr) == "object") {
		for (var key in attr) {
			e.setAttribute(key, attr[key]);
		}
	}
	return e;
}


function svgBox(title, x, y, text, color) {
	var w = 115;
	var c = [
		svgElement('rect', { x:1, y:1, width:w, height: 50, rx:10, ry:10, stroke:"black", 'stroke-width':1, fill: (color || "white")}),
		svgElement('text', { x:4, y:16, fill:"black", 'font-size':14}, title),
		svgElement('title', null, title)
	];
	if (text) {
		c.push(svgElement('text', { x: 10, y: 40, fill: "gray"}, text));
	}
	return svgElement('svg', {x: x, y: y, width: w + 2}, c);
}


function svgConnect(points, xlen, color) {
	var path = points.slice(1).reduce(function(acc,pt,i){
			return acc.concat('C', points[i][0]+xlen, points[i][1], pt[0]-xlen,pt[1], pt[0],pt[1])
		},['M',points[0][0],points[0][1]]);
	return svgElement('path', {d:path.join(' '), stroke:(color||"black"), 'stroke-width': 2, fill:'none', 'marker-end': 'url(#m)'});
}

function ProcGraph(){
	this.ids = {};
	this.elements = [];
}
ProcGraph.prototype.add = function(elem) {
	if (elem.id) { this.ids[elem.id] = elem; }
	elem.srcIds = elem.srcIds || [];
	this.elements.push(elem);
}

ProcGraph.prototype.build = function(svg, onclick) {
	var lanes = [0,0,0,0,0,0,0];
	var laneWidth = 180;
	var h = 80;
	svg.append(svgElement('marker', {id: "m", markerWidth:4, markerHeight:4, viewBox:"0 0 10 10", orient:"auto", refX:6, refY:5},
	                               svgElement('path', {d:"M 0 0 L 10 5 L 0 10 z", fill:"gray"})));
	this.elements.forEach(function(elem){
		var lane = elem.lane || 0;
		var y = lanes[lane] * h + 30;
		var x = lane * laneWidth + 4;
		elem.g = svgBox(elem.id, x, y, elem.text, elem.color);
		elem.inX = x;
		elem.inY = y + 25;
		elem.outX = x + 115;
		elem.outY = y + 25;
		if (onclick) {
			elem.g.addEventListener('click', function(e){onclick(elem, e);});
		}
		elem.init && elem.init(elem);
		svg.append(elem.g);
		lanes[lane] ++;
	});

	var ids = this.ids;
	this.elements.forEach(function(elem){
		elem.srcIds.forEach(function(srcId){
			var src = ids[srcId];
			if (src) {
				var color = src.connectColor;
				var points = [[src.outX, src.outY]];
				var dy = src.outY / h * (25/Math.max.apply(null, lanes));
				for (var l = src.lane + 1; l < elem.lane;l++) {
					var ty = src.outY + (elem.inY - src.outY) * ( (l-src.lane) / (elem.lane - src.lane) );
					if (lanes[l] * h + 10 > ty) {
						var y = ((ty+h / 2) / h | 0) * h  + dy;
						var x = l * laneWidth + laneWidth * 0.1;
						points.push([x, y], [x + laneWidth * 0.4, y]);
					}
				}
				points.push([elem.inX,elem.inY]);
				svg.append(svgConnect(points, 50, color));
			}
		});
	});

	var height = Math.max.apply(null, lanes) * h + h/2;
	svg.style.height = height;
	var vb = svg.getAttribute("viewBox").split(' ');
	vb[3] = height;
	svg.setAttribute("viewBox", vb.join(' '));
}

return ProcGraph;
})();
