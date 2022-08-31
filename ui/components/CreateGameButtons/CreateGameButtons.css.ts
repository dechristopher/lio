import { style } from "@vanilla-extract/css";
import { BaseButtonStyle } from "../Button/Button.css";

export const QuickGameButtonStyle = style([
	BaseButtonStyle,
	{
		fontSize: 24,
	},
]);

export const CreateGameButtonStyle = style([
	BaseButtonStyle,
	{
		fontFamily: "'Noto Sans', sans-serif !important",
		fontSize: 15.5,
	},
]);

export const Chin = style({
	backgroundColor: "#cca57b",
	padding: "2px 16px",
	borderRadius: 3,
});

