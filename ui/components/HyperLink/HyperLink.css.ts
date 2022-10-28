import { style } from "@vanilla-extract/css";

export const HyperLinkStyle = style({
	marginLeft: 3,
	color: "blue",
	textDecoration: "underline",
	selectors: {
		"&:visited": {
			color: "#551a8b",
		},
	},
});
