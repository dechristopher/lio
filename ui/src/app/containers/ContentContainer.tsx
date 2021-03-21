import React, {FC} from "react";

export const ContentContainer: FC = props => {
	return (
		<div className="mt-16 w-screen flex flex-col items-stretch overflow-x-hidden overflow-y-auto" style={{height: "calc(100vh - 4rem)"}}>
			{props.children}
		</div>
	)
}