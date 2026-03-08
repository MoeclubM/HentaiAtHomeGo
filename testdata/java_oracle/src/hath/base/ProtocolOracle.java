package hath.base;

import java.io.IOException;
import java.lang.reflect.Field;
import java.net.InetAddress;
import java.net.URL;
import java.nio.ByteBuffer;

import javax.net.ssl.HandshakeCompletedListener;
import javax.net.ssl.SSLSession;
import javax.net.ssl.SSLSocket;

public class ProtocolOracle {
	private static class DummyInputQueryHandler implements InputQueryHandler {
		public String queryString(String querytext) {
			return "";
		}
	}

	private static class DummyClient extends HentaiAtHomeClient {
		private boolean refreshSettingsCalled = false;
		private boolean startDownloaderCalled = false;
		private boolean setCertRefreshCalled = false;
		private DummyServerHandler dummyServerHandler;

		DummyClient() {
			super(new DummyInputQueryHandler(), new String[0]);
			dummyServerHandler = new DummyServerHandler(this);
		}

		public void run() {}

		public ServerHandler getServerHandler() {
			return dummyServerHandler;
		}

		public synchronized void startDownloader() {
			startDownloaderCalled = true;
		}

		public void setCertRefresh() {
			setCertRefreshCalled = true;
		}
	}

	private static class DummyServerHandler extends ServerHandler {
		private DummyClient owner;

		DummyServerHandler(DummyClient owner) {
			super(owner);
			this.owner = owner;
		}

		public boolean refreshServerSettings() {
			owner.refreshSettingsCalled = true;
			return true;
		}
	}

	private static class FakeSSLSocket extends SSLSocket {
		private final InetAddress inetAddress;

		FakeSSLSocket(String host) throws Exception {
			this.inetAddress = InetAddress.getByName(host);
		}

		public InetAddress getInetAddress() {
			return inetAddress;
		}

		public String[] getSupportedCipherSuites() { return new String[0]; }
		public String[] getEnabledCipherSuites() { return new String[0]; }
		public void setEnabledCipherSuites(String[] suites) {}
		public String[] getSupportedProtocols() { return new String[0]; }
		public String[] getEnabledProtocols() { return new String[0]; }
		public void setEnabledProtocols(String[] protocols) {}
		public SSLSession getSession() { return null; }
		public void addHandshakeCompletedListener(HandshakeCompletedListener listener) {}
		public void removeHandshakeCompletedListener(HandshakeCompletedListener listener) {}
		public void startHandshake() throws IOException {}
		public void setUseClientMode(boolean mode) {}
		public boolean getUseClientMode() { return false; }
		public void setNeedClientAuth(boolean need) {}
		public boolean getNeedClientAuth() { return false; }
		public void setWantClientAuth(boolean want) {}
		public boolean getWantClientAuth() { return false; }
		public void setEnableSessionCreation(boolean flag) {}
		public boolean getEnableSessionCreation() { return false; }
	}

	private static void setStaticField(Class<?> cls, String fieldName, Object value) throws Exception {
		Field field = cls.getDeclaredField(fieldName);
		field.setAccessible(true);
		field.set(null, value);
	}

	private static String escape(String value) {
		if(value == null) {
			return "";
		}

		return value.replace("\\", "\\\\").replace("\r", "\\r").replace("\n", "\\n").replace("\t", "\\t");
	}

	private static void emit(String key, String value) {
		System.out.println("ORACLE\t" + key + "\t" + escape(value));
	}

	private static String readBody(HTTPResponseProcessor processor) throws Exception {
		int remaining = processor.getContentLength();
		StringBuilder sb = new StringBuilder();

		while(remaining > 0) {
			ByteBuffer buffer = processor.getPreparedTCPBuffer();
			int len = buffer.remaining();
			byte[] bytes = new byte[len];
			buffer.get(bytes);
			sb.append(new String(bytes, "ISO-8859-1"));
			remaining -= len;
		}

		return sb.toString();
	}

	private static void handleParseRequest(String[] args) throws Exception {
		String request = args[1].equals("__NULL__") ? null : args[1];
		boolean localNetworkAccess = Boolean.parseBoolean(args[2]);

		HTTPResponse response = new HTTPResponse(null);
		response.parseRequest(request, localNetworkAccess);
		HTTPResponseProcessor processor = response.getHTTPResponseProcessor();

		emit("status", Integer.toString(response.getResponseStatusCode()));
		emit("head", Boolean.toString(response.isRequestHeadOnly()));
		emit("servercmd", Boolean.toString(response.isServercmd()));
		emit("contentType", processor.getContentType());
		emit("contentLength", Integer.toString(processor.getContentLength()));
		emit("header", processor.getHeader());
		emit("body", readBody(processor));
	}

	private static void handleConfiguredParseRequest(String[] args) throws Exception {
		String request = args[1].equals("__NULL__") ? null : args[1];
		boolean localNetworkAccess = Boolean.parseBoolean(args[2]);
		int clientID = Integer.parseInt(args[3]);
		String clientKey = args[4];
		String serverTime = args[5];

		setStaticField(Settings.class, "clientID", clientID);
		setStaticField(Settings.class, "clientKey", clientKey);
		Settings.updateSetting("server_time", serverTime);

		HTTPResponse response = new HTTPResponse(null);
		response.parseRequest(request, localNetworkAccess);
		HTTPResponseProcessor processor = response.getHTTPResponseProcessor();

		emit("status", Integer.toString(response.getResponseStatusCode()));
		emit("head", Boolean.toString(response.isRequestHeadOnly()));
		emit("servercmd", Boolean.toString(response.isServercmd()));
		emit("contentType", processor.getContentType());
		emit("contentLength", Integer.toString(processor.getContentLength()));
		emit("header", processor.getHeader());
		emit("processorClass", processor.getClass().getSimpleName());
		emit("body", processor instanceof HTTPResponseProcessorText ? readBody(processor) : "");
	}

	private static void handleServerResponse(String[] args) throws Exception {
		ServerResponse response = ServerResponse.getServerResponse(new URL(args[1]), null);
		String[] responseText = response.getResponseText();

		emit("status", Integer.toString(response.getResponseStatus()));
		emit("failCode", response.getFailCode());
		emit("failHost", response.getFailHost());
		emit("textCount", responseText == null ? "0" : Integer.toString(responseText.length));
		emit("text", responseText == null ? "" : String.join("\n", responseText));
	}

	private static void handleURLQuery(String[] args) throws Exception {
		String act = args[1];
		String add = args[2];
		int clientID = Integer.parseInt(args[3]);
		String clientKey = args[4];
		String serverTime = args[5];
		String rpcHost = args[6];
		String rpcPort = args[7];
		String rpcPath = args[8];

		setStaticField(Settings.class, "clientID", clientID);
		setStaticField(Settings.class, "clientKey", clientKey);
		setStaticField(Settings.class, "rpcServerCurrent", null);
		setStaticField(Settings.class, "rpcServerLastFailed", null);
		Settings.updateSetting("server_time", serverTime);
		Settings.updateSetting("rpc_server_port", rpcPort);
		Settings.updateSetting("rpc_path", rpcPath);
		if(!rpcHost.isEmpty()) {
			Settings.updateSetting("rpc_server_ip", rpcHost);
		}

		emit("query", ServerHandler.getURLQueryString(act, add));
		emit("url", ServerHandler.getServerConnectionURL(act, add).toString());
	}

	private static void handleServercmd(String[] args) throws Exception {
		String request = args[1];
		String rpcIP = args[2];
		int clientID = Integer.parseInt(args[3]);
		String clientKey = args[4];
		String serverTime = args[5];

		setStaticField(Settings.class, "clientID", clientID);
		setStaticField(Settings.class, "clientKey", clientKey);
		Settings.updateSetting("server_time", serverTime);
		Settings.updateSetting("rpc_server_ip", rpcIP);
		Settings.setActiveClient(null);

		DummyClient client = new DummyClient();
		Settings.setActiveClient(client);
		HTTPServer httpServer = new HTTPServer(client);
		HTTPSession session = new HTTPSession(new FakeSSLSocket(rpcIP), 1, false, httpServer);
		HTTPResponse response = new HTTPResponse(session);
		response.parseRequest(request, false);
		HTTPResponseProcessor processor = response.getHTTPResponseProcessor();

		emit("status", Integer.toString(response.getResponseStatusCode()));
		emit("head", Boolean.toString(response.isRequestHeadOnly()));
		emit("servercmd", Boolean.toString(response.isServercmd()));
		emit("contentType", processor.getContentType());
		emit("contentLength", Integer.toString(processor.getContentLength()));
		emit("header", processor.getHeader());
		emit("processorClass", processor.getClass().getSimpleName());
		emit("body", processor instanceof HTTPResponseProcessorText ? readBody(processor) : "");
		emit("refreshSettingsCalled", Boolean.toString(client.refreshSettingsCalled));
		emit("startDownloaderCalled", Boolean.toString(client.startDownloaderCalled));
		emit("setCertRefreshCalled", Boolean.toString(client.setCertRefreshCalled));
	}

	public static void main(String[] args) throws Exception {
		if(args.length < 1) {
			throw new IllegalArgumentException("missing command");
		}

		if(args[0].equals("parse-request")) {
			handleParseRequest(args);
		}
		else if(args[0].equals("parse-request-config")) {
			handleConfiguredParseRequest(args);
		}
		else if(args[0].equals("server-response")) {
			handleServerResponse(args);
		}
		else if(args[0].equals("url-query")) {
			handleURLQuery(args);
		}
		else if(args[0].equals("servercmd")) {
			handleServercmd(args);
		}
		else {
			throw new IllegalArgumentException("unknown command: " + args[0]);
		}
	}
}
