import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

@Composable
fun ThreadlineMessageComposer() {
    val body = ThreadlineTokens.Typography.body
    Text(
        text = "Message",
        fontSize = body.androidSp.sp,
        lineHeight = (body.androidSp * body.lineHeight).sp,
        modifier = Modifier.padding(ThreadlineTokens.Space.md.dp),
    )
}
