import { style } from "@vanilla-extract/css";
import { BaseButtonStyle } from "../Button/Button.css";

export const AboutContainerStyle = style({
	textAlign: "left",
	width: "20rem",
	display: "flex",
	flexDirection: "column",
});

export const AboutRedirect = style({
	cursor: "pointer",
	fontSize: "1.2em",
	fontWeight: "bold",
	marginBottom: 12,
});

export const AboutButtonGroup = style({
	display: "flex",
	justifyContent: "space-between",
	margin: "0 12px",
});

export const AboutButton = style([
	BaseButtonStyle,
	{
		fontSize: 24,
		fontWeight: 600,
	},
]);
