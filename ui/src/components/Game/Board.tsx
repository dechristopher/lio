import React, {FC} from "react";
import Octadground, {OctadgroundProps} from "react-octadground/octadground";

export const Board: FC<OctadgroundProps> = props => {
	return <Octadground {...props} />;
}