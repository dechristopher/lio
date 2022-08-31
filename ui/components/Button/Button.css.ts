import { style } from "@vanilla-extract/css";

export const BaseButtonStyle = style({
	alignItems: "center",
	backgroundColor: "#FFFFDF",
	borderRadius: ".25rem",
	boxShadow: "rgba(0, 0, 0, 0.02) 0 1px 3px 0",
	boxSizing: "border-box",
	color: "rgba(0, 0, 0, 0.85)",
	cursor: "pointer",
	display: "inline-flex",
	fontFamily:
		'system-ui, -appleSystem, system-ui, "Helvetica Neue", Helvetica, Arial, sansSerif',
	fontWeight: 600,
	justifyContent: "center",
	lineHeight: 1.25,
	margin: 0,
	minHeight: "3rem",
	padding: "calc(.175rem - 1px) calc(.8rem - 1px)",
	position: "relative",
	textDecoration: "none",
	transition: "all 250ms",
	userSelect: "none",
	WebkitUserSelect: "none",
	touchAction: "manipulation",
	verticalAlign: "baseline",
	width: "auto",
	borderWidth: 2,
	borderTopColor: "#767676",
	borderLeftColor: "#767676",
	borderBottomColor: "#212121",
	borderRightColor: "#212121",

	":hover": {
		borderColor: "rgba(0, 0, 0, 0.15)",
		boxShadow: "rgba(0, 0, 0, 0.1) 0 4px 12px",
		color: "rgba(0, 0, 0, 0.65)",
		transform: "translateY(-1px)",
	},

	":focus": {
		borderColor: "rgba(0, 0, 0, 0.15)",
		boxShadow: "rgba(0, 0, 0, 0.1) 0 4px 12px",
		color: "rgba(0, 0, 0, 0.65)",
	},

	":active": {
		backgroundColor: "#F0F0F1",
		borderColor: "rgba(0, 0, 0, 0.15)",
		boxShadow: "rgba(0, 0, 0, 0.06) 0 2px 4px",
		color: "rgba(0, 0, 0, 0.65)",
		transform: "translateY(0)",
	},
});
