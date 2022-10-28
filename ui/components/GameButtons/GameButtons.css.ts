import { style } from "@vanilla-extract/css";
import { BaseButtonStyle } from "../Button/Button.css";

export const QuickGameButtonStyle = style([
	BaseButtonStyle,
	{
		fontSize: 24,
	},
]);

export const QuickGameButtonGroup = style({
	margin: "0 12px",
});

export const Break = style({
	width: "100%",
	backgroundColor: "#000",
	height: 1.5,
	margin: "12px 0",
});

export const CreateGameButtonStyle = style([
	BaseButtonStyle,
	{
		fontSize: 15.5,
		fontWeight: "bold",
	},
]);
