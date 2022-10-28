import { style } from "@vanilla-extract/css";

export const BaseButtonStyle = style({
	borderWidth: 2,
	fontWeight: 600,
	lineHeight: 1.25,
	minHeight: "3rem",
	cursor: "pointer",
	userSelect: "none",
	alignItems: "center",
	position: "relative",
	borderRadius: ".25rem",
	textDecoration: "none",
	display: "inline-flex",
	boxSizing: "border-box",
	transition: "all 250ms",
	justifyContent: "center",
	WebkitUserSelect: "none",
	verticalAlign: "baseline",
	borderTopColor: "#767676",
	backgroundColor: "#FFFFDF",
	borderLeftColor: "#767676",
	touchAction: "manipulation",
	borderRightColor: "#212121",
	borderBottomColor: "#212121",
	color: "rgba(0, 0, 0, 0.85)",
	boxShadow: "rgba(0, 0, 0, 0.02) 0 1px 3px 0",
	padding: "calc(.175rem - 1px) calc(.8rem - 1px)",
	fontFamily:
		'system-ui, -appleSystem, system-ui, "Helvetica Neue", Helvetica, Arial, sansSerif',

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
