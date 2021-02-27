import React, {FC} from "react";
import ReactDOM from "react-dom";

export const EntryPoint: FC = () => {
	return <h1>Hello World</h1>
}

ReactDOM.render(<EntryPoint />, document.querySelector("#root"))

